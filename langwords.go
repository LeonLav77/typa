package main

// Programming vocabularies.
//
// Each source has two flavours. The bare list is identifiers only, which keeps
// the caps and punctuation modifiers meaningful and leaves per-key analysis
// comparable with an ordinary English run. The symbol list is what the code
// actually looks like — sigils, arrows, call parens — which is the harder and
// more realistic exercise.
//
// Tokens are whitespace-free by construction: the typing engine splits a test
// on spaces, so a token containing one would be dealt as two words and every
// word-level timing for it would be wrong. `TestLangTokensAreSingleTokens`
// enforces that.

// laravelWords is PHP and Laravel vocabulary as bare identifiers.
var laravelWords = []string{
	// language
	"public", "private", "protected", "function", "class", "return", "use",
	"namespace", "extends", "implements", "interface", "trait", "abstract",
	"static", "const", "new", "null", "true", "false", "array", "string",
	"int", "bool", "float", "void", "throw", "try", "catch", "finally",
	"foreach", "while", "match", "switch", "case", "break", "continue",
	"echo", "isset", "empty", "unset", "instanceof", "readonly", "enum",
	// laravel
	"artisan", "eloquent", "model", "controller", "middleware", "migration",
	"factory", "seeder", "schema", "blade", "route", "request", "response",
	"validate", "rules", "guard", "policy", "gate", "queue", "job", "event",
	"listener", "observer", "provider", "facade", "container", "collection",
	"builder", "relation", "belongs", "morph", "pivot", "scope", "casts",
	"fillable", "guarded", "timestamps", "migrate", "rollback", "seed",
	"tinker", "composer", "vendor", "config", "cache", "session", "cookie",
	"redirect", "view", "compact", "paginate", "chunk", "where", "first",
	"find", "create", "update", "delete", "save", "fresh", "with", "load",
	"auth", "user", "token", "sanctum", "passport", "livewire", "nova",
	"resource", "collect", "dispatch", "notify", "mail", "storage", "env",
}

// laravelSymbols is the same vocabulary as it is actually typed.
var laravelSymbols = []string{
	"$request", "$user", "$model", "$data", "$id", "$query", "$this->",
	"->get()", "->first()", "->where(", "->find(", "->create(", "->save()",
	"->update(", "->delete()", "->with(", "->all()", "->count()",
	"Model::class", "Route::get(", "Route::post(", "Schema::create(",
	"DB::table(", "Auth::user()", "Str::slug(", "Cache::get(",
	"function", "public", "private", "protected",
	"view(", "response()", "redirect(",
	"[]", "()", "{}", "=>", "::", "?->", "??", ";", "|", "&&",
	"'name'", "'email'", "'id'", "'required'", "'nullable'", "'string'",
	"App\\Models;", "namespace", "declare(strict_types=1);",
	"@param", "@return", "/**", "*/", "<?php", "$table->id();",
}

// goWords is Go keywords and stdlib idioms as bare identifiers.
var goWords = []string{
	// keywords
	"func", "var", "const", "type", "struct", "interface", "map", "chan",
	"go", "defer", "select", "range", "for", "if", "else", "switch", "case",
	"return", "break", "continue", "package", "import", "fallthrough", "goto",
	// the vocabulary of actual Go
	"error", "nil", "true", "false", "string", "int", "int64", "float64",
	"byte", "rune", "bool", "make", "new", "len", "cap", "append", "copy",
	"delete", "close", "panic", "recover", "print", "context", "handler",
	"server", "client", "request", "response", "writer", "reader", "buffer",
	"bytes", "strings", "errors", "fmt", "http", "json", "sql", "time",
	"sync", "mutex", "once", "group", "ticker", "timer", "duration",
	"marshal", "unmarshal", "encode", "decode", "scan", "query", "exec",
	"wrap", "sprintf", "printf", "println", "fatal", "test", "bench",
	"pointer", "slice", "value", "field", "method", "receiver", "embed",
	"goroutine", "channel", "buffered", "select", "worker", "pool",
}

// goSymbols is Go as it is actually typed.
var goSymbols = []string{
	":=", "!=", "==", "<-", "&&", "||", "...", "{}", "()", "[]", "*",
	"&", "%v", "%s", "%d", "%w", "%q", "func()", "error)", "(err",
	"err!=nil", "return", "nil,", "ctx", "context.Context", "t.Fatal(",
	"t.Errorf(", "fmt.Errorf(", "fmt.Println(", "log.Printf(",
	"err", "!=", "nil", "defer", "make(", "chan", "[]byte", "[]string",
	"map[string]", "struct{}", "interface{}", "*testing.T", "sync.Mutex",
	"http.Handler", "w.Write(", "json.Marshal(", "os.Exit(1)",
	"`json:\"id\"`", "//", "/*", "*/", "package", "main()",
}

// pythonWords is Python keywords and builtins as bare identifiers.
var pythonWords = []string{
	// keywords
	"def", "class", "return", "yield", "import", "from", "as", "pass",
	"raise", "try", "except", "finally", "with", "while", "for", "in",
	"if", "elif", "else", "lambda", "global", "nonlocal", "assert", "del",
	"async", "await", "not", "and", "or", "is", "none", "true", "false",
	// builtins and the vocabulary of actual Python
	"self", "init", "super", "print", "len", "range", "list", "dict",
	"set", "tuple", "str", "int", "float", "bool", "bytes", "type",
	"enumerate", "zip", "map", "filter", "sorted", "reversed", "sum",
	"min", "max", "abs", "round", "open", "format", "join", "split",
	"strip", "append", "extend", "insert", "remove", "items", "keys",
	"values", "update", "get", "pop", "index", "count", "sort",
	"module", "package", "decorator", "property", "static", "method",
	"args", "kwargs", "generator", "iterator", "comprehension", "context",
	"pytest", "fixture", "assert", "mock", "patch", "numpy", "pandas",
	"request", "response", "session", "model", "query", "schema",
}

// pythonSymbols is Python as it is actually typed.
var pythonSymbols = []string{
	"self.", "__init__", "__name__", "__main__", "()", "[]", "{}", ":",
	"->", "=>", "==", "!=", ">=", "<=", "+=", "-=", "*", "**", "//",
	"def", "return", "None", "True", "False", "self,", "*args",
	"**kwargs", "f\"\"", "'key'", "\"value\"", "[0]", "[-1]", "[:]",
	"len(", "range(", "print(", "dict(", "list(", "str(", "int(",
	"__name__", "open(", "Exception", "import", "from",
	"@property", "@staticmethod", "@pytest.fixture", "#",
	"os", "typing", "List[str]", "Dict[str,", "Optional[",
	"->None:", "assert", "ValueError(", "lambda",
}
