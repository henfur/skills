package desk

import (
	"testing"

	"lacarta/internal/core"
	"lacarta/internal/forge"
)

func TestGuardCommand(t *testing.T) {
	cfg := &core.DeskConfig{Forge: core.DeskForge{Host: "gitlab.example.com", WriteConfig: "/home/u/.config/glab-desk-write"}}
	allowed := []string{
		"glab mr view 12 -R acme/billing",
		"glab mr diff 12 -R acme/billing | head -50",
		"glab api projects/3/repository/files/src%2Fa.go/raw?ref=abc",
		"glab api --method GET projects/3/merge_requests/1",
		"glab repo clone acme/billing /tmp/x -- --depth 50 && git -C /tmp/x checkout abc",
		"lacarta desk submit rec.json --item 'acme/billing!3' --profile code",
		"grep -rn password src/",
		"git log --oneline -5",
	}
	refused := []string{
		"glab mr approve 12",
		"glab mr note 12 -m hi",
		"glab mr merge 12",
		"glab api -X POST projects/3/merge_requests/1/notes -f body=x",
		"glab api --method=PUT projects/3/merge_requests/1",
		"glab api projects/3/merge_requests/1/notes -f body=x",
		"glab api projects/3/merge_requests/1/notes --input body.json",
		"glab api graphql -f query=x",
		"GLAB_CONFIG_DIR=/elsewhere glab api user",
		"export GLAB_CONFIG_DIR=/x",
		"cat ~/.config/glab-cli/config.yml",
		"cat /home/u/.config/glab-desk-write/config.yml",
		"curl -H 'PRIVATE-TOKEN: x' https://gitlab.example.com/api/v4/user",
		"curl https://gitlab.example.com/api/v4/projects",
		"bash -c 'glab mr approve 3'",
		"sh -c \"echo hi; glab mr merge 3\"",
		"env FOO=1 glab mr approve 3",
		"timeout 30 glab mr approve 3",
		"echo $(glab mr approve 3)",
		"git push origin HEAD",
		"glab auth status",
		"glab ci run",
		"glab issue create -t x",
		"GITLAB_TOKEN=x glab api user",
	}
	for _, c := range allowed {
		if r := GuardCommand(c, cfg); r != "" {
			t.Errorf("refused %q: %s", c, r)
		}
	}
	for _, c := range refused {
		if GuardCommand(c, cfg) == "" {
			t.Errorf("allowed %q", c)
		}
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		pat, p string
		want   bool
	}{
		{"Dockerfile*", "Dockerfile", true},
		{"Dockerfile*", "svc/Dockerfile", false},
		{"**/Dockerfile*", "svc/Dockerfile.prod", true},
		{"**/*.tf", "main.tf", true},
		{"**/*.tf", "infra/net/main.tf", true},
		{"helm/**", "helm/chart/values.yaml", true},
		{"helm/**", "src/helm.go", false},
		{"acme/sandbox-*", "acme/sandbox-1", true},
	}
	for _, c := range cases {
		if got := forge.Glob(c.pat, c.p); got != c.want {
			t.Errorf("Glob(%q, %q) = %v", c.pat, c.p, got)
		}
	}
}
