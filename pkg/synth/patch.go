package synth

import (
	"fmt"
	"strings"
)

func newFile(path, content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = "+" + line + "\n"
	}

	return fmt.Sprintf(`diff --git a/%s b/%s
new file mode 100644
index 000000000..aaaabbbbc
--- /dev/null
+++ b/%s
@@ -0,0 +1,%d @@
%s`, path, path, path, len(lines), strings.Join(lines, ""))
}
