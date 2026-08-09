ALTER TABLE posts ADD COLUMN crossposted_from UUID REFERENCES posts(id);
