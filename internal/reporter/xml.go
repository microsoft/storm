package reporter

import "strings"

// isValidXMLChar reports whether r is a legal character in an XML 1.0 document,
// per the Char production of the XML 1.0 specification:
//
//	Char ::= #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]
//
// Notably this excludes the C0 control characters other than tab (#x9), line
// feed (#xA) and carriage return (#xD), which are the bytes commonly emitted by
// serial consoles and similar raw output sources.
func isValidXMLChar(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

// sanitizeXMLText removes characters that are illegal in XML 1.0 from s,
// preserving tab, line feed and carriage return. Test case output is copied
// verbatim into the JUnit report (into <system-out> CDATA and status message
// attributes); without this, raw control bytes such as NUL make the emitted
// document invalid XML 1.0, causing consumers (e.g. Azure DevOps) to reject the
// entire results file and silently drop the run's test results.
func sanitizeXMLText(s string) string {
	return strings.Map(func(r rune) rune {
		if isValidXMLChar(r) {
			return r
		}
		return -1 // drop the rune
	}, s)
}
