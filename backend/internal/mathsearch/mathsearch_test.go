package mathsearch

import (
	"strings"
	"testing"
)

func TestTokensCoverSymbolsAndCommands(t *testing.T) {
	got := Tokens(`The gradient ∇f and \alpha with ∫ f dx, ψ, \mathbb{R}`)
	for _, want := range []string{"nabla", "gradient", "alpha", "int", "integral", "psi", "mathbb"} {
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if Tokens("plain prose only") != "" {
		t.Error("prose must produce no math tokens")
	}
}

func TestExpandTerm(t *testing.T) {
	cases := map[string]string{"∇": "nabla", "α": "alpha", `\nabla`: "nabla", "∇f": "nabla f", "plain": "plain"}
	for in, want := range cases {
		if got := ExpandTerm(in); got != want {
			t.Errorf("ExpandTerm(%q) = %q, want %q", in, got, want)
		}
	}
}
