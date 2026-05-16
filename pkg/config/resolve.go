package config

// Resolve picks a value by precedence.
func Resolve[T any](flagChanged bool, flag T, cfg, env *T, def T) T {
	switch {
	case flagChanged:
		return flag
	case cfg != nil:
		return *cfg
	case env != nil:
		return *env
	default:
		return def
	}
}
