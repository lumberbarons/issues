package conventions

import (
	"fmt"
	"strings"
)

// TemplateSections returns the body-template section headers for a type, in
// order. Bugs describe a problem and a fix; enhancements and tasks a goal
// and an approach.
func TemplateSections(issueType string) []string {
	if issueType == "bug" {
		return []string{"### Where", "### Problem", "### Fix", "### Done when"}
	}
	return []string{"### Where", "### Goal", "### Approach", "### Done when"}
}

// TemplateSkeleton renders the empty template for --edit: headers with
// blank slots, "Done when" seeded with one checklist item.
func TemplateSkeleton(issueType string) string {
	var b strings.Builder
	for _, h := range TemplateSections(issueType) {
		b.WriteString(h)
		b.WriteString("\n\n")
		if h == "### Done when" {
			b.WriteString("- [ ] \n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// StripEmptySections removes template sections whose body is blank, so a
// half-filled skeleton posts clean. Non-template content is preserved
// verbatim.
func StripEmptySections(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "### ") {
			// Collect the section: header plus everything up to the next header.
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(lines[j], "### ") {
				j++
			}
			if sectionHasContent(lines[i+1 : j]) {
				out = append(out, lines[i:j]...)
			}
			i = j
			continue
		}
		out = append(out, line)
		i++
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func sectionHasContent(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || t == "- [ ]" {
			continue
		}
		return true
	}
	return false
}

// DiscoveredFrom is the body line linking discovered work to its origin.
func DiscoveredFrom(n int) string {
	return fmt.Sprintf("Discovered while working on #%d", n)
}

// EpicTitlePrefix is the cosmetic title prefix the tool adds to parent
// issues. Epic-ness itself is defined by having sub-issues.
const EpicTitlePrefix = "Epic: "
