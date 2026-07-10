package plexams

// programmAndAncode is the (program, primussAncode) key used to translate a Primuss
// registration to its internal ZPA ancode.
type programmAndAncode struct {
	Program string
	Ancode  int
}

// resolveAncodes translates one student registration's Primuss ancode into the pair
// (primussAncode, zpaAncode). The Primuss ancode is what the student registered on
// (external, per-program); the ZPA ancode is the internal one. They differ for
// MUC.DAI/external exams and — in principle, e.g. after a Prüfungsamt correction —
// may differ for FK07 exams too. Equality is never assumed: the mapping
// primussToZpa (built from the connected exams' PrimussAncodes) is authoritative;
// when it has no entry the ZPA ancode falls back to the Primuss ancode.
func resolveAncodes(program string, primussAncode int, primussToZpa map[programmAndAncode]int) (primuss, zpa int) {
	if zpaAncode, ok := primussToZpa[programmAndAncode{program, primussAncode}]; ok {
		return primussAncode, zpaAncode
	}
	return primussAncode, primussAncode
}
