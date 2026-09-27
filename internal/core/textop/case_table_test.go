package textop_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

// These literal expectations are derived from the table in doc.go, without
// calling any OT implementation to construct them. Each row starts with the
// named pair and compares first-component lengths 1:2, 2:2, 3:2. Suffixes make
// the operations context-valid and expose residual consumption and draining.
func TestCaseTable(t *testing.T) {
	type composeRow struct {
		pair       string
		a, b, want [3]string
	}
	for _, row := range []composeRow{
		{"RR", [3]string{`[1,"X"]`, `[2,"X"]`, `[3,"X"]`}, [3]string{`[2,"Y"]`, `[2,"Y",1]`, `[2,"Y",2]`}, [3]string{`[1,"XY"]`, `[2,"YX"]`, `[2,"Y",1,"X"]`}},
		{"RI", [3]string{`[1]`, `[2]`, `[3]`}, [3]string{`["YY",1]`, `["YY",2]`, `["YY",3]`}, [3]string{`["YY",1]`, `["YY",2]`, `["YY",3]`}},
		{"RD", [3]string{`[1,"XXX"]`, `[2,"XXX"]`, `[3,"XXX"]`}, [3]string{`[-2,2]`, `[-2,3]`, `[-2,4]`}, [3]string{`["XX",-1]`, `["XXX",-2]`, `[-2,1,"XXX"]`}},
		{"IR", [3]string{`["x",1]`, `["xx",1]`, `["xxx",1]`}, [3]string{`[2,"Y"]`, `[2,"Y",1]`, `[2,"Y",2]`}, [3]string{`["x",1,"Y"]`, `["xxY",1]`, `["xxYx",1]`}},
		{"II", [3]string{`["x"]`, `["xx"]`, `["xxx"]`}, [3]string{`["YY",1]`, `["YY",2]`, `["YY",3]`}, [3]string{`["YYx"]`, `["YYxx"]`, `["YYxxx"]`}},
		{"ID", [3]string{`["x",2]`, `["xx",2]`, `["xxx",2]`}, [3]string{`[-2,1]`, `[-2,2]`, `[-2,3]`}, [3]string{`[-1,1]`, `[2]`, `["x",2]`}},
		{"DR", [3]string{`[-1,2]`, `[-2,2]`, `[-3,2]`}, [3]string{`[2]`, `[2]`, `[2]`}, [3]string{`[-1,2]`, `[-2,2]`, `[-3,2]`}},
		{"DI", [3]string{`[-1]`, `[-2]`, `[-3]`}, [3]string{`["YY"]`, `["YY"]`, `["YY"]`}, [3]string{`["YY",-1]`, `["YY",-2]`, `["YY",-3]`}},
		{"DD", [3]string{`[-1,2]`, `[-2,2]`, `[-3,2]`}, [3]string{`[-2]`, `[-2]`, `[-2]`}, [3]string{`[-3]`, `[-4]`, `[-5]`}},
	} {
		for i, relation := range []string{"shorter", "equal", "longer"} {
			t.Run("compose/"+row.pair+"/"+relation, func(t *testing.T) {
				a, b := mustOp(t, row.a[i]), mustOp(t, row.b[i])
				got, err := textop.Compose(a, b)
				if err != nil {
					t.Fatal(err)
				}
				if err := equalOp(got, []byte(row.want[i])); err != nil {
					t.Fatal(err)
				}
				base := ""
				for range a.BaseLen() {
					base += "a"
				}
				d := mustDoc(t, base)
				mid, err := applyChecked(d, a)
				if err != nil {
					t.Fatal(err)
				}
				sequential, err := applyChecked(mid, b)
				if err != nil {
					t.Fatal(err)
				}
				combined, err := applyChecked(d, got)
				if err != nil {
					t.Fatal(err)
				}
				if combined.String() != sequential.String() {
					t.Fatal("compose paths differ")
				}
			})
		}
	}
	type transformRow struct {
		pair         string
		a, b, ap, bp [3]string
	}
	for _, row := range []transformRow{
		{"RR", [3]string{`[1,"x",2]`, `[2,"x",1]`, `[3,"x"]`}, [3]string{`[2,"Y",1]`, `[2,"Y",1]`, `[2,"Y",1]`}, [3]string{`[1,"x",3]`, `[2,"x",2]`, `[4,"x"]`}, [3]string{`[3,"Y",1]`, `[3,"Y",1]`, `[2,"Y",2]`}},
		{"RI", [3]string{`[1]`, `[2]`, `[3]`}, [3]string{`["YY",1]`, `["YY",2]`, `["YY",3]`}, [3]string{`[3]`, `[4]`, `[5]`}, [3]string{`["YY",1]`, `["YY",2]`, `["YY",3]`}},
		{"RD", [3]string{`[1,"x",2]`, `[2,"x",1]`, `[3,"x"]`}, [3]string{`[-2,1]`, `[-2,1]`, `[-2,1]`}, [3]string{`["x",1]`, `["x",1]`, `[1,"x"]`}, [3]string{`[-1,1,-1,1]`, `[-2,2]`, `[-2,2]`}},
		{"IR", [3]string{`["x",2]`, `["xx",2]`, `["xxx",2]`}, [3]string{`[2]`, `[2]`, `[2]`}, [3]string{`["x",2]`, `["xx",2]`, `["xxx",2]`}, [3]string{`[3]`, `[4]`, `[5]`}},
		{"II", [3]string{`["x"]`, `["xx"]`, `["xxx"]`}, [3]string{`["YY"]`, `["YY"]`, `["YY"]`}, [3]string{`["x",2]`, `["xx",2]`, `["xxx",2]`}, [3]string{`[1,"YY"]`, `[2,"YY"]`, `[3,"YY"]`}},
		{"ID", [3]string{`["x",2]`, `["xx",2]`, `["xxx",2]`}, [3]string{`[-2]`, `[-2]`, `[-2]`}, [3]string{`["x"]`, `["xx"]`, `["xxx"]`}, [3]string{`[1,-2]`, `[2,-2]`, `[3,-2]`}},
		{"DR", [3]string{`[-1,2]`, `[-2,1]`, `[-3]`}, [3]string{`[2,"Y",1]`, `[2,"Y",1]`, `[2,"Y",1]`}, [3]string{`[-1,3]`, `[-2,2]`, `[-2,1,-1]`}, [3]string{`[1,"Y",1]`, `["Y",1]`, `["Y"]`}},
		{"DI", [3]string{`[-1]`, `[-2]`, `[-3]`}, [3]string{`["YY",1]`, `["YY",2]`, `["YY",3]`}, [3]string{`[2,-1]`, `[2,-2]`, `[2,-3]`}, [3]string{`["YY"]`, `["YY"]`, `["YY"]`}},
		{"DD", [3]string{`[-1,2]`, `[-2,1]`, `[-3]`}, [3]string{`[-2,1]`, `[-2,1]`, `[-2,1]`}, [3]string{`[1]`, `[1]`, `[-1]`}, [3]string{`[-1,1]`, `[1]`, `[]`}},
	} {
		for i, relation := range []string{"shorter", "equal", "longer"} {
			t.Run("transform/"+row.pair+"/"+relation, func(t *testing.T) {
				a, b := mustOp(t, row.a[i]), mustOp(t, row.b[i])
				as, bs := encoded(a), encoded(b)
				ap, bp, err := textop.Transform(a, b)
				if err != nil {
					t.Fatal(err)
				}
				if err := equalOp(ap, []byte(row.ap[i])); err != nil {
					t.Fatal(err)
				}
				if err := equalOp(bp, []byte(row.bp[i])); err != nil {
					t.Fatal(err)
				}
				d := mustDoc(t, strings.Repeat("a", a.BaseLen()))
				da, err := applyChecked(d, a)
				if err != nil {
					t.Fatal(err)
				}
				db, err := applyChecked(d, b)
				if err != nil {
					t.Fatal(err)
				}
				left, err := applyChecked(da, bp)
				if err != nil {
					t.Fatal(err)
				}
				right, err := applyChecked(db, ap)
				if err != nil {
					t.Fatal(err)
				}
				if left.String() != right.String() {
					t.Fatal("transform paths differ")
				}
				if ap.BaseLen() != b.TargetLen() || bp.BaseLen() != a.TargetLen() || ap.TargetLen() != bp.TargetLen() {
					t.Fatal("transform length identities differ")
				}
				if !bytes.Equal(as, encoded(a)) || !bytes.Equal(bs, encoded(b)) {
					t.Fatal("transform mutated operand")
				}
			})
		}
	}
}
