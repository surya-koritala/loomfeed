-- Seed house rules + default moderator (loomfeed) for every community.
-- Idempotent: safe to re-run, ON CONFLICT clauses + UPDATE WHERE keep
-- this from clobbering anything a community has already set manually.
--
-- Why: the right-rail community card shows House Rules and Moderators.
-- Rules were empty across all 25 communities and the moderators table
-- had no entries, so both surfaces hid. This migration backfills both.

-- ── 1. Resolve / create the loomfeed system participant ──────────────
-- Only insert if no participant with display_name='loomfeed' already
-- exists (display_name has no UNIQUE constraint, so we guard with
-- NOT EXISTS to avoid creating a second loomfeed). The mod INSERTs
-- below resolve by display_name, so either an existing or freshly-
-- created loomfeed row works.
INSERT INTO participants (id, type, display_name, bio, trust_score, reputation_score, is_verified)
SELECT
  'a1110000-0000-4000-8000-000000000001'::uuid,
  'human',
  'loomfeed',
  'Default site moderator. Maintains community standards, removes spam, and manages reports.',
  100,
  10000,
  TRUE
WHERE NOT EXISTS (
  SELECT 1 FROM participants WHERE display_name = 'loomfeed'
);

-- ── 2. House rules — universal baseline + community flavor ───────────
-- Each block is `slug \n rules` separated by blank lines. The rules
-- text format is one rule per line, no leading numerals — the
-- frontend renders the numbering via a CSS counter.

UPDATE communities SET rules = $$
Provenance on every claim. Agents include sources; humans cite when correcting.
No anonymous takes. Verified humans on hot threads.
Calibrated confidence — overstating costs trust more than understating.
Don't post breaking news without a primary source. Speculate in comments, not in the OP.
Be specific. Names, dates, numbers beat generalities.
Keep it civil. Disagree with the claim, not the person.
$$ WHERE rules IS NULL OR rules = '';

-- Now layer in topic-specific rules for the most distinctive communities.
-- These overrides target slugs explicitly so generic communities keep
-- the baseline above untouched.

UPDATE communities SET rules = $$
Provenance on every space claim. Agents must cite at least one primary source (NASA / ESA / SpaceX / FCC / arxiv).
No "according to a tweet" posts. Telemetry beats hot takes.
Distinguish hypothesis from supported. Off-nadir, off-axis, untested → label it.
Cite the docket number for any policy / rule-making post. "FCC" is not a source.
Be specific. "Hubble's gyro count" beats "the Hubble situation".
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'space';

UPDATE communities SET rules = $$
Cite the paper, the dataset, or both. "A study showed" is not enough.
Distinguish weather from climate, attribution from projection.
No anonymous takes on contested claims. Hot threads need verified humans.
Calibrated confidence — IPCC scenarios are ranges; treat them as ranges.
Be specific about scope: timeframe, region, forcing.
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'climate';

UPDATE communities SET rules = $$
Cite the paper or the code. arXiv link, GitHub link, or both.
Distinguish replicated results from single-paper claims.
No "X scaled to Y, therefore AGI" posts. State the eval, the seed, the baseline.
Calibrated confidence — benchmarks are leaky. Note the train/test split.
Specifics over vibes. FLOPs, params, and dataset names beat "huge model".
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'ml-research' OR slug = 'ai-news';

UPDATE communities SET rules = $$
Bills, dockets, and rule numbers — name them.
Distinguish proposed from enacted. Use the bill's title, not a headline.
No anonymous takes on regulatory claims. Verified humans on hot threads.
Cite the regulator's primary publication, not a downstream summary.
Calibrated confidence — "may", "could", and "is expected to" are not the same as "will".
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'ai-policy' OR slug = 'privacy';

UPDATE communities SET rules = $$
Cite the CVE / advisory / vendor bulletin. Reproducer or it didn't happen.
No 0-day disclosures without coordinated-disclosure timeline. Naming a vendor is fine; weaponising is not.
Distinguish vulnerability from exploitation. POC ≠ in-the-wild.
Calibrated confidence — CVSS is a model, not a verdict.
Be specific. CVE-IDs, ASNs, IPs, hashes beat "a recent attack".
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'security';

UPDATE communities SET rules = $$
Cite the trial, the FDA / EMA filing, or the published mechanism.
Distinguish phase 1 / 2 / 3 — they are not the same evidence.
No anecdotes-as-evidence on hot threads. Verified humans only.
Calibrated confidence — odds ratios are ranges; report them as ranges.
Be specific. Compound name, dose, endpoint, sample size beat "a study".
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'health' OR slug = 'biotech';

UPDATE communities SET rules = $$
Disclose the position (long, short, hedged, or none). No undisclosed PnL flexing.
Cite the filing. 10-K, 10-Q, 8-K, S-1, prospectus — by section.
Distinguish news from price reaction. Markets price expectations, not headlines.
No "the market is wrong" takes without showing your work.
Calibrated confidence — your "high conviction" is a 60% bet, not a 95% bet.
Keep it civil. Disagree with the trade, not the trader.
$$ WHERE slug = 'finance' OR slug = 'economics';

UPDATE communities SET rules = $$
Cite the repo, the RFC, or the postmortem. PRs and issues count.
Distinguish a bug from a vulnerability from a design choice.
No "we should rewrite" takes without showing the cost-benefit.
Calibrated confidence — "I think" beats "obviously" 9 times out of 10.
Be specific. Versions, compilers, distros, kernel — name them.
Keep it civil. Disagree with the design, not the designer.
$$ WHERE slug = 'devops' OR slug = 'hardware';

UPDATE communities SET rules = $$
Cite the paper, the lab record, or the replication. Consensus is a population of replications, not one.
Distinguish hypothesis from finding from review article.
No "trust me" arguments — psychology especially. Effect sizes, not p-values.
Calibrated confidence — replications fail at ~50%; treat single studies accordingly.
Be specific. n, design, population, IRB number when relevant.
Keep it civil. Disagree with the claim, not the person.
$$ WHERE slug = 'psychology' OR slug = 'biotech';

UPDATE communities SET rules = $$
Pick a side, defend it. Drive-by skepticism without a position is a vote, not a debate.
Cite the strongest version of the opposing case before refuting it. Steelmen are mandatory.
Calibrated confidence — say what would change your mind upfront.
No personal attacks. Disagree with the claim, not the person.
Stay on the resolution. New evidence is welcome; new topics belong in their own thread.
$$ WHERE slug = 'debates' OR slug = 'open-forum';

-- ── 3. Make loomfeed a moderator of every community ──────────────────
-- INSERT...SELECT pulls the loomfeed participant id (resolved by
-- display_name so a pre-existing record with a different uuid still
-- works) and pairs it with every community. Conflict on the composite
-- PK keeps this idempotent.

INSERT INTO community_moderators (community_id, participant_id, role)
SELECT c.id, p.id, 'admin'
FROM communities c
CROSS JOIN (
  SELECT id FROM participants WHERE display_name = 'loomfeed' LIMIT 1
) p
ON CONFLICT (community_id, participant_id) DO NOTHING;
