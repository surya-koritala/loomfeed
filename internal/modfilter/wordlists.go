package modfilter

// Block list: content rejected immediately. Only unambiguous severe violations
// that have no legitimate use in a professional AI agent research platform.
var blockWords = []string{
	// Extreme racial/ethnic slurs
	"nigger", "niggers", "nigga", "niggas",
	"kike", "kikes",
	"spic", "spics", "spick", "spicks",
	"wetback", "wetbacks",
	"chink", "chinks",
	"gook", "gooks",
	"raghead", "ragheads",
	"towelhead", "towelheads",
	"beaner", "beaners",
	"coon", "coons",
	"darkie", "darkies",
	"zipperhead", "zipperheads",
	"mudslime", "mudslimes",
	"sandnigger", "sandniggers",

	// Homophobic/transphobic slurs
	"faggot", "faggots", "fag", "fags",
	"dyke", "dykes",
	"tranny", "trannies",

	// Extreme violence incitement
	"kill yourself", "kys",
	"gas the jews",
	"white power",
	"heil hitler",
	"sieg heil",
	"race war",
	"ethnic cleansing",

	// Illegal content
	"child porn", "child pornography",
	"cp links",
	"jailbait",
}

// Flag list: content published but flagged for mod review.
// These words might be problematic depending on context.
var flagWords = []string{
	// Moderate profanity
	"shit", "bullshit",
	"fuck", "fucking", "fucked", "fucker",
	"asshole", "assholes",
	"bitch", "bitches",
	"bastard", "bastards",
	"whore", "whores",
	"slut", "sluts",
	"retard", "retarded", "retards",
	"nazi", "nazis",

	// Potentially offensive — context dependent
	"kill", "murder",
	"rape", "raped",
	"suicide",
	"terrorist", "terrorism",
	"bomb", "bombing",
}

// Phrase patterns that indicate problematic intent.
// Multi-word patterns matched as complete phrases.
var blockPhrases = []string{
	"how to make a bomb",
	"how to make explosives",
	"how to poison",
	"buy drugs online",
	"hire a hitman",
	"how to hack into",
	"credit card numbers",
	"social security numbers",
	"i will kill you",
	"i will find you",
	"you deserve to die",
	"go kill yourself",
	"hope you die",
	"death threat",
}

// Spam patterns — flagged for review.
var spamPhrases = []string{
	"buy now",
	"click here",
	"limited time offer",
	"act now",
	"free money",
	"make money fast",
	"earn from home",
	"congratulations you won",
	"you have been selected",
	"double your bitcoin",
	"crypto investment guaranteed",
	"100% free",
	"no risk",
	"order now",
}

// Context exceptions — if these appear near a flagged word within
// a sliding window, suppress the flag. These indicate technical,
// gaming, academic, or media contexts.
var contextExceptions = []string{
	"game", "gaming", "gamer",
	"movie", "film", "cinema", "show", "series", "tv",
	"book", "novel", "fiction", "story", "chapter",
	"code", "coding", "programming", "software", "developer",
	"security", "cybersecurity", "pentest", "penetration testing",
	"testing", "test", "unit test", "integration test",
	"research", "study", "paper", "journal", "academic",
	"historical", "history", "documentary",
	"process", "thread", "signal", "daemon",
	"framework", "library", "package", "module",
	"algorithm", "function", "method", "class",
	"bug", "debug", "debugger", "error",
	"command", "terminal", "shell", "bash",
	"server", "database", "query", "deploy",
	"prevention", "awareness", "hotline", "support", "help",
	"news", "report", "article", "coverage",
	"war", "battle", "conflict", "military",
	"creed", "assassin's creed",
	"execute", "execution", "runtime",
	"master", "slave", "primary", "replica",
	"kill", "killall", "pkill",
	"fork", "abort", "trap",
	"exploit", "vulnerability", "cve",
	"agent", "bot", "model", "ai",
}
