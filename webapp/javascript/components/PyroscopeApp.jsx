import React, { useState, useEffect } from "react";
import { connect } from "react-redux";
import "react-dom";

import Modal from "react-modal";

import { withShortcut } from "react-keybind";
import { bindActionCreators } from "redux";
import FlameGraphRenderer from "./FlameGraphRenderer";
import TimelineChart from "./TimelineChart";
import ShortcutsModal from "./ShortcutsModal";
import Header from "./Header";
import Footer from "./Footer";
import { fetchNames } from "../redux/actions";

// See docs here: https://github.com/flot/flot/blob/master/API.md
const flotOptions = {
  margin: {
    top: 0,
    left: 0,
    bottom: 0,
    right: 0,
  },
  selection: {
    mode: "x",
  },
  grid: {
    borderWidth: 1,
    margin: {
      left: 16,
      right: 16,
    },
  },
  yaxis: {
    show: false,
    min: 0,
  },
  points: {
    show: false,
    radius: 0.1,
  },
  lines: {
    show: false,
    steps: true,
    lineWidth: 1.0,
  },
  bars: {
    show: true,
    fill: true,
  },
  xaxis: {
    mode: "time",
    timezone: "browser",
    reserveSpace: false,
  },
};

const modalStyle = {
  overlay: {
    backgroundColor: "rgba(0,0,0,0.75)",
  },
  content: {
    background: "#222",
    border: "1px solid #111",
  },
};

const initialState = {
  shortcutsModalOpen: false,
};

function PyroscopeApp(props) {
  const { actions, shortcut, timeline } = props;
  const [state, setState] = useState(initialState);
  useEffect(() => {
    shortcut.registerShortcut(
      showShortcutsModal,
      ["shift+?"],
      "Shortcuts",
      "Show Keyboard Shortcuts Modal"
    );
  }, []);

  const showShortcutsModal = () => {
    setState({ shortcutsModalOpen: true });
  };

  const closeShortcutsModal = () => {
    setState({ shortcutsModalOpen: false });
  };

  const flotData = timeline
    ? [timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
    : [];

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChart
          id="timeline-chart"
          options={flotOptions}
          data={flotData}
          width="100%"
          height="100px"
        />
        <FlameGraphRenderer />
        <Modal
          isOpen={state.shortcutsModalOpen}
          style={modalStyle}
          appElement={document.getElementById("root")}
        >
          <div className="modal-close-btn" onClick={closeShortcutsModal} />
          <ShortcutsModal closeModal={closeShortcutsModal} />
        </Modal>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchNames,
    },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(withShortcut(PyroscopeApp));
