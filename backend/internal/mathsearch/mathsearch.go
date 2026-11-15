// Package mathsearch makes mathematical notation searchable. The default FTS5
// tokenizer drops symbols such as the nabla and integral signs and treats Greek
// letters and their LaTeX commands as unrelated words; this package maps both to
// shared plain-word names.
package mathsearch

import (
	"regexp"
	"sort"
	"strings"
)

var symbolNames = map[rune][]string{
	'α': {"alpha"}, 'β': {"beta"}, 'γ': {"gamma"}, 'δ': {"delta"}, 'ε': {"epsilon"}, 'ζ': {"zeta"}, 'η': {"eta"}, 'θ': {"theta"},
	'ι': {"iota"}, 'κ': {"kappa"}, 'λ': {"lambda"}, 'μ': {"mu"}, 'ν': {"nu"}, 'ξ': {"xi"}, 'π': {"pi"}, 'ρ': {"rho"}, 'σ': {"sigma"},
	'τ': {"tau"}, 'υ': {"upsilon"}, 'φ': {"phi"}, 'χ': {"chi"}, 'ψ': {"psi"}, 'ω': {"omega"},
	'Γ': {"gamma"}, 'Δ': {"delta"}, 'Θ': {"theta"}, 'Λ': {"lambda"}, 'Ξ': {"xi"}, 'Π': {"pi"}, 'Σ': {"sigma"}, 'Φ': {"phi"}, 'Ψ': {"psi"}, 'Ω': {"omega"},
	'∇': {"nabla", "gradient"}, '∂': {"partial"}, '∫': {"int", "integral"}, '∮': {"oint", "integral"}, '∑': {"sum", "summation"}, '∏': {"prod", "product"},
	'√': {"sqrt", "root"}, '∞': {"infty", "infinity"}, '±': {"pm"}, '×': {"times"}, '⋅': {"cdot"}, '·': {"cdot"},
	'≤': {"leq", "le"}, '≥': {"geq", "ge"}, '≠': {"neq", "ne"}, '≈': {"approx"}, '≡': {"equiv"}, '∼': {"sim"}, '∝': {"propto"},
	'∈': {"in", "element"}, '∉': {"notin"}, '⊂': {"subset"}, '⊆': {"subseteq"}, '∪': {"cup", "union"}, '∩': {"cap", "intersection"},
	'∀': {"forall"}, '∃': {"exists"}, '∅': {"emptyset", "varnothing"}, '→': {"rightarrow", "to"}, '←': {"leftarrow"}, '⇒': {"implies", "rightarrow"}, '⇔': {"iff"},
	'ℝ': {"mathbb", "reals"}, 'ℂ': {"mathbb", "complex"}, 'ℕ': {"mathbb", "naturals"}, 'ℤ': {"mathbb", "integers"}, 'ℚ': {"mathbb", "rationals"},
	'ℏ': {"hbar"}, '⟨': {"langle", "bra"}, '⟩': {"rangle", "ket"}, '⊗': {"otimes"}, '⊕': {"oplus"}, '∧': {"wedge", "land"}, '∨': {"vee", "lor"},
	'¬': {"neg", "lnot"}, '⊥': {"perp"}, '∥': {"parallel"}, '†': {"dagger"}, '°': {"degree"},
}

// byCommand maps a LaTeX command name (the first name of a symbol) to all of that symbol's names.
var byCommand = func() map[string][]string {
	m := map[string][]string{}
	for _, names := range symbolNames {
		if _, taken := m[names[0]]; !taken {
			m[names[0]] = names
		}
	}
	return m
}()

var commandRE = regexp.MustCompile(`\\([A-Za-z]{2,24})`)

// Tokens returns plain-word names for every mathematical symbol and LaTeX command in text.
func Tokens(text string) string {
	seen := map[string]struct{}{}
	for _, r := range text {
		for _, n := range symbolNames[r] {
			seen[n] = struct{}{}
		}
	}
	for _, m := range commandRE.FindAllStringSubmatch(text, -1) {
		name := strings.ToLower(m[1])
		seen[name] = struct{}{}
		for _, n := range byCommand[name] {
			seen[n] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	joined := strings.Join(out, " ")
	if len(joined) > 4000 {
		joined = joined[:4000]
	}
	return joined
}

// ExpandTerm turns a query term containing symbols or LaTeX commands into the
// plain words used by the index. Terms without notation are returned unchanged.
func ExpandTerm(term string) string {
	term = strings.TrimLeft(term, `\`)
	var b strings.Builder
	changed := false
	for _, r := range term {
		if names, ok := symbolNames[r]; ok {
			b.WriteString(" " + names[0] + " ")
			changed = true
			continue
		}
		b.WriteRune(r)
	}
	if !changed {
		return term
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
