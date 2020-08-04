// +build "go1.11" "go1.12"

package errcheck

import (
	"fmt"
	"strings"
)

func fmtTags(tags []string) string {
	return fmt.Sprintf("-tags=%s", strings.Join(tags, " "))
}
