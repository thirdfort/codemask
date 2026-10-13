package codemask

import (
	"path"
	"strings"
)

type language struct {
	name string
	// stateless languages have no construct that spans lines, so any line can be
	// lexed without seeing the ones before it, and a partial-context diff is
	// still exact.
	stateless bool
	newLexer  func() lexer
}

var (
	dq      = strDelim{open: `"`, close: `"`, escape: true}
	sq      = strDelim{open: `'`, close: `'`, escape: true}
	cBlock  = [][2]string{{"/*", "*/"}}
	slashes = []string{"//"}
)

var goConfig = &langConfig{
	lineComments:  slashes,
	blockComments: cBlock,
	strings:       []strDelim{dq, sq, {open: "`", close: "`", multiline: true}},
}

func jsConfig(jsx bool) *langConfig {
	return &langConfig{
		lineComments:  slashes,
		blockComments: cBlock,
		strings:       []strDelim{dq, sq, {open: "`", close: "`", escape: true, multiline: true, interp: []string{"${"}}},
		jsRegex:       true,
		jsx:           jsx,
	}
}

var pythonConfig = &langConfig{
	lineComments: []string{"#"},
	strings: []strDelim{
		{open: `"""`, close: `"""`, escape: true, multiline: true},
		{open: `'''`, close: `'''`, escape: true, multiline: true},
		dq, sq,
	},
}

var sqlConfig = &langConfig{
	lineComments:  []string{"--"},
	blockComments: cBlock,
	strings: []strDelim{
		{open: `'`, close: `'`, doubled: true, multiline: true},
		{open: `"`, close: `"`, doubled: true, multiline: true},
	},
	dollarQuote: true,
}

var hclConfig = &langConfig{
	lineComments:  []string{"#", "//"},
	blockComments: cBlock,
	strings:       []strDelim{{open: `"`, close: `"`, escape: true, interp: []string{"${", "%{"}}},
	heredoc:       true,
	blockLabels:   true,
}

var cssConfig = &langConfig{blockComments: cBlock, strings: []strDelim{dq, sq}}

var scssConfig = &langConfig{lineComments: slashes, blockComments: cBlock, strings: []strDelim{dq, sq}}

var protoConfig = &langConfig{lineComments: slashes, blockComments: cBlock, strings: []strDelim{dq, sq}}

var jsonConfig = &langConfig{strings: []strDelim{dq}}

var goModConfig = &langConfig{lineComments: slashes, strings: []strDelim{dq}}

func clikeLang(name string, cfg *langConfig, stateless bool) *language {
	return &language{name: name, stateless: stateless, newLexer: func() lexer { return newClike(cfg) }}
}

var languages = map[string]*language{
	".go":     clikeLang("go", goConfig, false),
	".ts":     clikeLang("typescript", jsConfig(false), false),
	".mts":    clikeLang("typescript", jsConfig(false), false),
	".cts":    clikeLang("typescript", jsConfig(false), false),
	".tsx":    clikeLang("tsx", jsConfig(true), false),
	".js":     clikeLang("javascript", jsConfig(true), false),
	".jsx":    clikeLang("javascript", jsConfig(true), false),
	".mjs":    clikeLang("javascript", jsConfig(true), false),
	".cjs":    clikeLang("javascript", jsConfig(true), false),
	".py":     clikeLang("python", pythonConfig, false),
	".sql":    clikeLang("sql", sqlConfig, false),
	".tf":     clikeLang("hcl", hclConfig, false),
	".tfvars": clikeLang("hcl", hclConfig, false),
	".hcl":    clikeLang("hcl", hclConfig, false),
	".css":    clikeLang("css", cssConfig, false),
	".scss":   clikeLang("scss", scssConfig, false),
	".less":   clikeLang("scss", scssConfig, false),
	".proto":  clikeLang("proto", protoConfig, false),
	".json":   clikeLang("json", jsonConfig, true),
	".yaml":   {name: "yaml", newLexer: func() lexer { return newYAML() }},
	".yml":    {name: "yaml", newLexer: func() lexer { return newYAML() }},
	".sh":     {name: "shell", newLexer: func() lexer { return newShell(false) }},
	".bash":   {name: "shell", newLexer: func() lexer { return newShell(false) }},
	".zsh":    {name: "shell", newLexer: func() lexer { return newShell(false) }},
}

var (
	goModLang      = clikeLang("gomod", goModConfig, true)
	dockerfileLang = &language{name: "dockerfile", newLexer: func() lexer { return newShell(true) }}
)

// detectLanguage picks a lexer from the file name, or nil if there is none.
func detectLanguage(p string) *language {
	base := path.Base(p)
	switch {
	case base == "go.mod" || base == "go.work":
		return goModLang
	case isDockerfile(base):
		return dockerfileLang
	default:
		return languages[strings.ToLower(path.Ext(base))]
	}
}
