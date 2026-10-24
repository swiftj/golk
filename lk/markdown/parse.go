package markdown

import (
	"errors"

	"github.com/swiftj/golk/lk/blocks"
)

// parse parses src into blocks (LK_MARKDOWN.md §2, §5). Ids are set only
// where the input writes them: {#id} and ::unsupported id=….
func parse(src string) (blocks.List, error) {
	return nil, errors.New("markdown: parse not implemented")
}
