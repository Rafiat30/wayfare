package route

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Wayfare-labs/wayfare/asset"
)

// TestToQuoteJSONCarriesKind pins that ToQuoteJSON never drops Kind. The
// field existed on Quote before this test was written but was silently
// dropped at the wire boundary, which is exactly the conflation issue #181
// exists to fix: a client had no way to tell an on-chain DEX quote from an
// anchor's own SEP-38 rate once either reached JSON.
func TestToQuoteJSONCarriesKind(t *testing.T) {
	for _, kind := range []Kind{KindDEX, KindAnchorSEP38} {
		t.Run(string(kind), func(t *testing.T) {
			q := &Quote{
				Kind:          kind,
				Description:   "USDC -> NGNC",
				Source:        "stellar-dex",
				ReceiveAmount: decimal.RequireFromString("100"),
				EffectiveRate: decimal.RequireFromString("1000"),
				LossPct:       decimal.RequireFromString("1.5"),
				Verdict:       VerdictGood,
			}
			got := ToQuoteJSON(q)
			if got.Kind != string(kind) {
				t.Errorf("Kind = %q, want %q", got.Kind, kind)
			}
		})
	}
}

// TestToQuoteJSONNilIsNil guards the existing nil-safety of ToQuoteJSON,
// which the new field must not disturb.
func TestToQuoteJSONNilIsNil(t *testing.T) {
	if got := ToQuoteJSON(nil); got != nil {
		t.Errorf("ToQuoteJSON(nil) = %+v, want nil", got)
	}
}

// TestToCorridorJSONPropagatesQuoteKind builds a minimal LadderResult with a
// DEX-priced rung and asserts the kind survives all the way to both the
// per-rung quote and the recommended quote on the rendered CorridorJSON —
// the actual shape a client reads over HTTP.
func TestToCorridorJSONPropagatesQuoteKind(t *testing.T) {
	send, recv := asset.USDC(), asset.NGNC()
	q := Quote{
		Kind:          KindDEX,
		Description:   "USDC -> NGNC",
		Source:        "stellar-dex",
		SendAsset:     send,
		SendAmount:    decimal.RequireFromString("100"),
		ReceiveAsset:  recv,
		ReceiveAmount: decimal.RequireFromString("129000"),
		EffectiveRate: decimal.RequireFromString("1290"),
		LossPct:       decimal.RequireFromString("4.46"),
		Verdict:       VerdictFair,
		QuotedAt:      time.Now(),
	}

	lr := &LadderResult{
		Request: LadderRequest{SendAsset: send, ReceiveAsset: recv, ReferenceBase: "USD", ReferenceQuote: "NGN"},
		Rungs: []Rung{{
			SendAmount: decimal.RequireFromString("100"),
			Result:     &Result{Quotes: []Quote{q}, Integrity: IntegrityDirect},
		}},
		Integrity:       IntegrityDirect,
		ReferenceMid:    decimal.RequireFromString("1350"),
		ReferenceSource: "exchangerate-api",
		Recommended:     &q,
		RecommendedSize: decimal.RequireFromString("100"),
	}

	out := ToCorridorJSON(lr, "USD/NGN")

	if out.Rungs[0].Quote == nil {
		t.Fatal("expected the rung to carry a quote")
	}
	if out.Rungs[0].Quote.Kind != string(KindDEX) {
		t.Errorf("rung quote kind = %q, want %q", out.Rungs[0].Quote.Kind, KindDEX)
	}
	if out.Recommended == nil {
		t.Fatal("expected a recommended quote")
	}
	if out.Recommended.Kind != string(KindDEX) {
		t.Errorf("recommended kind = %q, want %q", out.Recommended.Kind, KindDEX)
	}

	// And on the actual wire bytes, not just the Go struct — a field present
	// on the struct but dropped by a stray json tag would pass every check
	// above and still ship broken.
	raw, err := json.Marshal(out.Recommended)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wire["kind"] != "dex" {
		t.Errorf(`wire "kind" = %v, want "dex"`, wire["kind"])
	}
}
