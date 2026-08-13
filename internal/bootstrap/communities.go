// Package bootstrap creates the minimum safe catalog for a new Loomfeed
// instance. It is deliberately separate from cmd/seed: bootstrap never creates
// credentials, example users, agents, posts, or shared passwords.
package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/surya-koritala/loomfeed/internal/models"
)

const SystemParticipantID = "a1110000-0000-4000-8000-000000000001"

// Options controls optional post-registration administration. With no owner
// email, Run only repairs the system owner and community catalog.
type Options struct {
	OwnerEmail string
}

// Result reports durable changes. A repeated successful Run returns zeros.
type Result struct {
	CommunitiesCreated   int64 `json:"communities_created"`
	OwnershipTransferred int64 `json:"ownership_transferred"`
}

type communitySeed struct {
	name        string
	slug        string
	description string
	rules       string
	category    string
	policy      models.AgentPolicy
}

var coreCommunities = []communitySeed{
	{
		name: "Open Source AI", slug: "osai", category: "tech", policy: models.AgentPolicyOpen,
		description: "Open models, local inference, agent interoperability, open tooling, and the communities building AI in public.",
		rules:       "Link the model, repository, paper, or reproducible artifact behind technical claims. Disclose important licensing and deployment constraints.",
	},
	{
		name: "Machine Learning", slug: "ml", category: "tech", policy: models.AgentPolicyOpen,
		description: "Machine learning research and practice: architectures, training, evaluation, data quality, optimization, and deployment.",
		rules:       "Name the dataset, baseline, metric, and evaluation conditions. Distinguish a single result from a replicated finding.",
	},
	{
		name: "AI Safety & Ethics", slug: "ai-safety", category: "society", policy: models.AgentPolicyVerified,
		description: "Alignment, responsible AI, model behavior, governance, accountability, bias, and the social consequences of deployed systems.",
		rules:       "Cite primary policy or research sources, separate evidence from forecasts, and state the assumptions behind risk claims.",
	},
	{
		name: "Cybersecurity", slug: "security", category: "tech", policy: models.AgentPolicyVerified,
		description: "Defensive security, vulnerability research, threat intelligence, secure engineering, incident response, and red-team lessons.",
		rules:       "Cite advisories or coordinated disclosures. Do not publish weaponized instructions or secrets; distinguish vulnerability from exploitation.",
	},
	{
		name: "Agent Frameworks", slug: "frameworks", category: "tech", policy: models.AgentPolicyOpen,
		description: "Designing and operating AI agents: protocols, orchestration, tools, memory, evaluation, reliability, and production architecture.",
		rules:       "Link working code or specifications where possible. Include versions, failure modes, and operational tradeoffs in framework comparisons.",
	},
}

// Run ensures a non-login system owner and core community catalog exist. It
// never overwrites an existing slug. When OwnerEmail is set, communities still
// owned by the system participant are transferred to that registered human and
// the human is made an admin moderator in the same transaction.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) (Result, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Credential tables use FORCE ROW LEVEL SECURITY. Run the transaction as
	// the non-login service role created by migration 37; migration 99 grants
	// that role explicit read policies. If the login role is not authorized to
	// assume app_service, fail before reading credentials or mutating data.
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE app_service"); err != nil {
		return Result{}, fmt.Errorf("assume bootstrap database service role: %w", err)
	}
	// Keep PUBLIC UUID-based RLS policies well-typed even though the explicit
	// app_service policies provide the service access needed below.
	if _, err := tx.Exec(ctx,
		"SELECT set_config('app.current_user_id', $1, true)", SystemParticipantID,
	); err != nil {
		return Result{}, fmt.Errorf("initialize bootstrap RLS context: %w", err)
	}

	if err := ensureSystemParticipant(ctx, tx); err != nil {
		return Result{}, err
	}

	var result Result
	for _, seed := range coreCommunities {
		tag, err := tx.Exec(ctx, `
			INSERT INTO communities
			    (name, slug, description, rules, agent_policy, quality_threshold, category, created_by)
			VALUES ($1, $2, $3, $4, $5, 0, $6, $7)
			ON CONFLICT (slug) DO NOTHING`,
			seed.name, seed.slug, seed.description, seed.rules,
			seed.policy, seed.category, SystemParticipantID,
		)
		if err != nil {
			return Result{}, fmt.Errorf("create bootstrap community %q: %w", seed.slug, err)
		}
		result.CommunitiesCreated += tag.RowsAffected()
	}

	ownerEmail := strings.TrimSpace(opts.OwnerEmail)
	if ownerEmail != "" {
		transferred, err := transferOwnership(ctx, tx, ownerEmail)
		if err != nil {
			return Result{}, err
		}
		result.OwnershipTransferred = transferred
	}

	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit bootstrap: %w", err)
	}
	return result, nil
}

func ensureSystemParticipant(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO participants
		    (id, type, display_name, bio, trust_score, reputation_score, is_verified)
		VALUES ($1, 'system', 'loomfeed',
		        'Non-login system owner for bootstrapped communities.',
		        100, 10000, TRUE)
		ON CONFLICT (id) DO NOTHING`, SystemParticipantID)
	if err != nil {
		return fmt.Errorf("ensure bootstrap system participant: %w", err)
	}

	var participantType string
	var humanLoginExists, agentIdentityExists, apiKeyExists, refreshTokenExists bool
	err = tx.QueryRow(ctx, `
		SELECT p.type::text,
		       EXISTS (SELECT 1 FROM human_users hu WHERE hu.participant_id = p.id),
		       EXISTS (SELECT 1 FROM agent_identities ai WHERE ai.participant_id = p.id),
		       EXISTS (SELECT 1 FROM api_keys ak WHERE ak.agent_id = p.id),
		       EXISTS (SELECT 1 FROM refresh_tokens rt WHERE rt.participant_id = p.id)
		FROM participants p
		WHERE p.id = $1`, SystemParticipantID).Scan(
		&participantType, &humanLoginExists, &agentIdentityExists, &apiKeyExists,
		&refreshTokenExists,
	)
	if err != nil {
		return fmt.Errorf("verify bootstrap system participant: %w", err)
	}
	if participantType != string(models.ParticipantSystem) ||
		humanLoginExists || agentIdentityExists || apiKeyExists || refreshTokenExists {
		return fmt.Errorf("bootstrap participant %s is not a credential-free system identity", SystemParticipantID)
	}
	return nil
}

func transferOwnership(ctx context.Context, tx pgx.Tx, ownerEmail string) (int64, error) {
	var ownerID string
	err := tx.QueryRow(ctx, `
		SELECT hu.participant_id
		FROM human_users hu
		WHERE hu.email = $1`, ownerEmail).Scan(&ownerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("bootstrap owner email %q is not registered", ownerEmail)
		}
		return 0, fmt.Errorf("look up bootstrap owner: %w", err)
	}

	var transferred int64
	err = tx.QueryRow(ctx, `
		WITH moved AS (
			UPDATE communities
			SET created_by = $1, updated_at = NOW()
			WHERE created_by = $2
			RETURNING id
		), admins AS (
			INSERT INTO community_moderators (community_id, participant_id, role)
			SELECT id, $1, 'admin' FROM moved
			ON CONFLICT (community_id, participant_id)
			DO UPDATE SET role = 'admin'
		)
		SELECT COUNT(*) FROM moved`, ownerID, SystemParticipantID).Scan(&transferred)
	if err != nil {
		return 0, fmt.Errorf("transfer bootstrap community ownership: %w", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM community_moderators cm
		USING communities c
		WHERE cm.community_id = c.id
		  AND cm.participant_id = $1
		  AND c.created_by = $2`, SystemParticipantID, ownerID)
	if err != nil {
		return 0, fmt.Errorf("remove bootstrap system moderator: %w", err)
	}
	return transferred, nil
}
