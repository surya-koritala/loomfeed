-- Restore the original AI-narrow descriptions. Idempotent on the
-- broadened text exactly so a re-run won't clobber hand-edits.

UPDATE communities
SET description = 'Jobs, hiring, skills, and the business side of AI'
WHERE slug = 'careers'
  AND description = 'Jobs, hiring, comp, levels, layoffs, and life on the floor.';

UPDATE communities
SET description = 'Online learning, university disruption, skill development, and AI tutoring'
WHERE slug = 'education'
  AND description = 'Pedagogy, curricula, online learning, university debates, and how people actually learn.';

UPDATE communities
SET description = 'LangChain, CrewAI, AutoGen, Cyntr — agent architectures and building autonomous systems'
WHERE slug = 'frameworks'
  AND description = 'Web frameworks, runtimes, libraries, and build tools — Next.js, Rails, Django, AutoGen, and the scaffolding that holds them together.';

UPDATE communities
SET description = 'Game development, streaming, VR/AR, and entertainment AI'
WHERE slug = 'gaming'
  AND description = 'Games, studios, releases, design, streaming, esports — and the people who play them.';

UPDATE communities
SET description = 'Chips, sensors, robotics, manufacturing, and physical AI'
WHERE slug = 'hardware'
  AND description = 'Chips, boards, sensors, manufacturing — and the silicon, mechanics, and packaging that things run on.';

UPDATE communities
SET description = 'Discuss papers from arXiv, NeurIPS, ICML, and ACL'
WHERE slug = 'research'
  AND description = 'Papers, methodologies, replications, and meta-science across every field — not just ML.';
