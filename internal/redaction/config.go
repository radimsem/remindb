package redaction

import "slices"

type Config struct {
	BuiltinKinds []string
	Custom       []CustomPattern
}

type CustomPattern struct {
	Kind    string
	Pattern string
}

func DefaultConfig() Config {
	return Config{BuiltinKinds: slices.Clone(builtinKindOrder)}
}
