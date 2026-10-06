package presence

import (
	"strings"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// linkScheme is what every published link starts with.
const linkScheme = "https://"

// fallbackLabel is the label for a host that is not in hostLabels.
const fallbackLabel = "View repository"

// hostLabels is the label of the button, by the host of the link. A label is
// at most 32 characters, which is Discord's limit.
var hostLabels = map[string]string{
	"github.com":    "View on GitHub",
	"gitlab.com":    "View on GitLab",
	"bitbucket.org": "View on Bitbucket",
	"codeberg.org":  "View on Codeberg",
}

// button is the button for a session's repository link, or none when the
// session has no link. The link was validated by the adapter and by the
// host; one that is not https is left out all the same.
func button(link string) domain.Button {
	rest, ok := strings.CutPrefix(link, linkScheme)
	if !ok {
		return domain.Button{}
	}
	host, _, _ := strings.Cut(rest, "/")
	label, known := hostLabels[host]
	if !known {
		label = fallbackLabel
	}
	return domain.Button{Label: label, URL: link}
}
