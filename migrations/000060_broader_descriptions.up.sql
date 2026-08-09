-- Six community descriptions still scoped to AI/ML even though those
-- topics have their own dedicated communities (ai-news, ai-safety,
-- machine-learning, osai). Broaden them to match the actual subject
-- so /a/careers feels like careers, /a/research feels like research,
-- and so on.
--
-- Idempotent — each UPDATE matches the exact current description so
-- a re-run is a no-op and any hand-edits done after this lands won't
-- be overwritten.

UPDATE communities
SET description = 'Jobs, hiring, comp, levels, layoffs, and life on the floor.'
WHERE slug = 'careers'
  AND description = 'Jobs, hiring, skills, and the business side of AI';

UPDATE communities
SET description = 'Pedagogy, curricula, online learning, university debates, and how people actually learn.'
WHERE slug = 'education'
  AND description = 'Online learning, university disruption, skill development, and AI tutoring';

UPDATE communities
SET description = 'Web frameworks, runtimes, libraries, and build tools — Next.js, Rails, Django, AutoGen, and the scaffolding that holds them together.'
WHERE slug = 'frameworks'
  AND description = 'LangChain, CrewAI, AutoGen, Cyntr — agent architectures and building autonomous systems';

UPDATE communities
SET description = 'Games, studios, releases, design, streaming, esports — and the people who play them.'
WHERE slug = 'gaming'
  AND description = 'Game development, streaming, VR/AR, and entertainment AI';

UPDATE communities
SET description = 'Chips, boards, sensors, manufacturing — and the silicon, mechanics, and packaging that things run on.'
WHERE slug = 'hardware'
  AND description = 'Chips, sensors, robotics, manufacturing, and physical AI';

UPDATE communities
SET description = 'Papers, methodologies, replications, and meta-science across every field — not just ML.'
WHERE slug = 'research'
  AND description = 'Discuss papers from arXiv, NeurIPS, ICML, and ACL';
