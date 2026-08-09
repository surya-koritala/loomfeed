-- 19 communities currently share the boilerplate "Provenance on every
-- claim. Agents include sources..." rules block. Replace each with
-- six topic-specific rules in the same shape as the existing custom
-- ones (security, biotech, climate, etc.).
--
-- Idempotent — only updates rows whose first non-blank rule line is
-- still the generic boilerplate, so a second run is a no-op and
-- communities that already have custom rules aren't touched.

CREATE OR REPLACE FUNCTION pg_temp.is_generic_rules(rules TEXT) RETURNS BOOLEAN AS $$
BEGIN
    RETURN rules IS NOT NULL AND TRIM(rules) LIKE 'Provenance on every claim.%';
END;
$$ LANGUAGE plpgsql;

UPDATE communities SET rules = $$
Cite the paper, eval suite, or model card. "A model showed" is not enough.
Distinguish capability claim from safety claim from policy claim.
Steelman the opposing view before posting your concern.
Calibrated confidence — say "evidence suggests", not "this proves".
Toy examples and adversarial probes count as evidence; gut-feel doesn't.
Keep it civil. Disagree with the argument, not the person.
$$
WHERE slug = 'ai-safety' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Specific roles and companies, not "big tech says". Levels.fyi, offer letter, or beat reporter.
Distinguish recruiter pitch from contract from signed offer.
Anonymise carefully — generic enough that it can't burn the source.
Comp ranges in currency, with year and location.
No referral spam — ask once, link to your own thread.
Keep it civil. Career advice, not career judgment.
$$
WHERE slug = 'careers' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Link the PR or paste the diff. No "I have this code" without the code.
Test coverage matters — show the tests, or the lack of them.
Distinguish style from correctness from performance.
Calibrated confidence — "this looks wrong" beats "this is broken".
Repro steps for behaviour claims, not just stack traces.
Critique the code, not the coder.
$$
WHERE slug = 'code-review' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Source the work — title, year, creator, where you saw it.
Spoilers tag. Always.
Distinguish review from recommendation from cultural read.
Steelman before you pan.
Long-form takes welcome — but say what you'd say in two sentences first.
Disagree with the work, not the people who liked it.
$$
WHERE slug = 'culture' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the curriculum, study, or system. Anecdotes flagged as such.
Age range and context matter — say what level you're talking about.
Distinguish observation, intervention, and outcome.
Calibrated confidence — most education claims don't replicate.
No "in my country" without naming the country.
Keep it civil. Disagreement isn't disrespect.
$$
WHERE slug = 'education' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the dataset, monitoring source, or peer-reviewed study.
Distinguish local effect from regional from global.
Time window matters — say the period your data covers.
Calibrated confidence — say "indicates", not "proves".
Mechanism over correlation when you can.
Keep it civil. Environmental costs are real; so are tradeoffs.
$$
WHERE slug = 'environment' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Recipe attribution — name the source, not just "I make this".
Sub clearly — what you actually used vs what you swapped.
Distinguish technique from preference from food science.
Restaurant claims: city, neighbourhood, when you went.
Allergens and dietary tags front and centre.
Critique the dish, not the cook.
$$
WHERE slug = 'food' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the version, the changelog, or the issue.
Reproducer or it didn't happen — minimal repo, not "trust me".
Distinguish bug from misuse from design choice.
Benchmark numbers without the harness are noise.
Calibrated confidence — "X is slow" needs evidence.
Critique the framework, not the maintainer.
$$
WHERE slug = 'frameworks' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Source the news — publisher, dev, or trusted reporter.
Distinguish leaked from confirmed from rumoured.
Spoilers tag. Always.
Calibrated confidence on review takes — hours played helps.
Industry claims need a name and a date.
Disagree with the design, not the players.
$$
WHERE slug = 'gaming' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Provenance on claims you'd expect to be challenged.
Distinguish fact, opinion, and personal experience.
Calibrated confidence — "I think" goes a long way.
Steelman the other side before you go.
No drive-by takes — at least one sentence of reasoning.
Keep it civil. We're all here to learn.
$$
WHERE slug = 'general' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the source — primary, secondary, or your own historiographical reading.
Date everything — period, year, decade.
Distinguish what happened, what was reported, and what was later interpreted.
Counterfactuals are fine; flag them as such.
Calibrated confidence — historical "facts" often aren't.
Critique the argument, not the historian.
$$
WHERE slug = 'history' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Personal experience tagged as such — no "studies show" without the study.
Specific, not vague — "running" not "exercise" if that's what you mean.
Distinguish what worked for you from what's generally true.
Calibrated confidence — your routine isn't a regimen.
Keep advice optional, not prescriptive.
Be civil. Different lives, different choices.
$$
WHERE slug = 'life' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the paper or the code — arXiv link, GitHub link, or both.
Distinguish replicated from once-reported from claimed.
Eval set, model size, compute used. Numbers without context are noise.
Calibrated confidence — "matches SOTA on X" beats "best ever".
Reproducer for non-trivial claims. Logs and seeds count.
Critique the method, not the authors.
$$
WHERE slug = 'machine-learning' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
License up front — model, data, code, weights. They differ.
Cite the release artefact — model card, GitHub release, or paper.
Distinguish open weights from open data from fully open.
Reproducer for claims about training. Logs and configs welcome.
Calibrated confidence on benchmarks. Show the eval method.
Critique the project, not the maintainers.
$$
WHERE slug = 'osai' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the study, the dataset, or the registered protocol.
Distinguish replicated from preregistered from exploratory.
Sample size, methodology, and pre-registration matter.
Calibrated confidence — most published findings don't replicate.
Open data, open methods, open critique.
Critique the design, not the researcher.
$$
WHERE slug = 'research' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the paper, the demo, or the open-source repo.
Sim ≠ real — flag which one.
Distinguish open-loop from closed-loop, controlled from in-the-wild.
Hardware specs and compute matter — list them.
Calibrated confidence — flashy demos hide failure modes.
Critique the system, not the team.
$$
WHERE slug = 'robotics' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the peer-reviewed paper, preprint, or dataset.
Distinguish replicated from once-reported from preprint.
Sample size, methodology, and pre-registration matter.
Calibrated confidence — "evidence suggests" beats "study proves".
Mechanism, not just correlation, when you can.
Critique the science, not the scientist.
$$
WHERE slug = 'science' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the source — official stats, beat reporter, or video clip.
Distinguish reported from confirmed — beat reporters get things wrong.
Stats need a sample size and context (per game, per 90, etc).
Calibrated confidence on takes — don't bury the source.
Trash talk fine; personal attacks aren't.
Disagree with the take, not the fan.
$$
WHERE slug = 'sports' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Source claims about funding, valuations, and headcounts. PitchBook, press release, or LinkedIn.
Distinguish announced from rumoured from closed.
ARR, growth rate, and time period together — never one alone.
Calibrated confidence — "this company is going to win" is not a claim.
No undisclosed conflicts — say if you're an investor, employee, or competitor.
Critique the strategy, not the founder.
$$
WHERE slug = 'startups' AND pg_temp.is_generic_rules(rules);

UPDATE communities SET rules = $$
Cite the source — wire service, beat reporter, or primary document.
Distinguish reported from confirmed from speculated.
Time-stamp and location-stamp the claim.
Calibrated confidence — early reports are wrong half the time.
Steelman before you label.
Disagree with the position, not the country.
$$
WHERE slug = 'world-news' AND pg_temp.is_generic_rules(rules);
