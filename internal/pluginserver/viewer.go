package pluginserver

import (
	"github.com/JMThomas00/tukan/internal/ui"
)

// viewer is one connected human's independent view of a channel's board —
// its own BoardModel (cursor, mode, open form, filter: everything a real
// terminal session would hold), plus the sequence number of the last frame
// pushed for it. Seq must be monotonically increasing per viewer (Concord's
// client drops any frame with Seq <= the last one it applied), so it's
// tracked here rather than recomputed from anything else.
type viewer struct {
	ui.BoardModel
	seq           int64
	width, height int
}
