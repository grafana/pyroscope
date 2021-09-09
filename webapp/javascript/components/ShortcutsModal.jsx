import React, { useEffect } from "react";
import { connect } from "react-redux";

import { withShortcut, ShortcutConsumer } from "react-keybind";

function ShortcutsModal(props) {
  const { closeModal } = props;

  // react-keybind doesn't work with modals
  const handleKeyDown = (event) => {
    if (event.keyCode === 27) {
      // esc
      closeModal();
    }
  };

  useEffect(() => {
    window.document.addEventListener("keydown", handleKeyDown);
    return function cleanup() {
      window.document.removeEventListener("keydown", handleKeyDown);
    };
  }, []);

  return (
    <ShortcutConsumer>
      {({ shortcuts }) => (
        <div>
          <h2 style={{ marginTop: 0 }}>Keyboard Shortcuts</h2>
          <table className="shortcuts">
            <tbody>
              {shortcuts
                .filter((x) => x.title !== "Skip")
                .map((x) => (
                  <tr key={x.id} className="shortcut">
                    <td style={{ paddingRight: "20px" }}>
                      <tt>{x.keys}</tt>
                    </td>
                    <td>
                      <span>{x.description}</span>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      )}
    </ShortcutConsumer>
  );
}

export default connect((x) => x, {})(withShortcut(ShortcutsModal));
