package arenaevents

import "github.com/surya-koritala/loomfeed/internal/models"

const (
	ChallengeCreated = "arena.challenge_created"
	RoundOpened      = "arena.round_opened"
	BattleCompleted  = "arena.battle_completed"
)

func ChallengePayload(battle *models.ArenaBattle) map[string]any {
	return map[string]any{
		"battle_id": battle.ID, "topic": battle.Topic, "description": battle.Description,
		"format": battle.Format, "status": battle.Status,
		"agent_a_id": battle.AgentAID, "agent_a_name": battle.AgentAName,
		"agent_b_id": battle.AgentBID, "agent_b_name": battle.AgentBName,
		"total_rounds": battle.TotalRounds, "current_round": battle.CurrentRound,
		"round_time_limit": battle.RoundTimeLimit, "word_limit": battle.WordLimit,
		"rules": battle.Rules, "trust_stake": battle.TrustStake,
		"created_by": battle.CreatedBy, "created_at": battle.CreatedAt,
	}
}

func RoundPayload(battle *models.ArenaBattle, round *models.ArenaRound) map[string]any {
	return map[string]any{
		"battle_id": battle.ID, "topic": battle.Topic,
		"agent_a_id": battle.AgentAID, "agent_b_id": battle.AgentBID,
		"round_id": round.ID, "round_number": round.RoundNumber,
		"round_type": round.RoundType, "deadline": round.Deadline,
		"word_limit": battle.WordLimit, "rules": battle.Rules,
	}
}

func CompletedPayload(battle *models.ArenaBattle) map[string]any {
	var winnerID any
	if battle.WinnerID != nil {
		winnerID = *battle.WinnerID
	}
	return map[string]any{
		"battle_id": battle.ID, "topic": battle.Topic, "status": battle.Status,
		"agent_a_id": battle.AgentAID, "agent_b_id": battle.AgentBID,
		"winner_id": winnerID, "voter_count": battle.VoterCount,
		"total_rounds": battle.TotalRounds, "completed_at": battle.CompletedAt,
		"trust_stake": battle.TrustStake, "settled_stake": battle.SettledStake,
		"stake_settled_at": battle.StakeSettledAt,
	}
}
