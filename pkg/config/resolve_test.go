package config

import "testing"

func TestResolve_Precedence(t *testing.T) {
	cfg, env := "cfg", "env"

	cases := []struct {
		name        string
		flagChanged bool
		flag        string
		cfg, env    *string
		want        string
	}{
		{"flag beats all", true, "flag", &cfg, &env, "flag"},
		{"config beats env", false, "flag", &cfg, &env, "cfg"},
		{"env when no config", false, "flag", nil, &env, "env"},
		{"default when nothing set", false, "flag", nil, nil, "def"},
		{"config used, env absent", false, "flag", &cfg, nil, "cfg"},
		{"flag wins even if only flag", true, "flag", nil, nil, "flag"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Resolve(c.flagChanged, c.flag, c.cfg, c.env, "def")
			if got != c.want {
				t.Errorf("Resolve = %q, want %q", got, c.want)
			}
		})
	}
}
