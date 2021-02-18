import { connect } from "react-redux";
import "react-dom";

import ReactFlot from "react-flot";
import "react-flot/flot/jquery.flot.time.min";
import "react-flot/flot/jquery.flot.selection.min";
import "react-flot/flot/jquery.flot.crosshair.min";
import { bindActionCreators } from "redux";
import { setDateRange, receiveJSON } from "../redux/actions";

class TimelineChart extends ReactFlot {
  componentDidMount() {
    this.draw();
    $(`#${this.props.id}`).bind("plotselected", (event, ranges) => {
      this.props.actions.setDateRange(
        Math.round(ranges.xaxis.from / 1000),
        Math.round(ranges.xaxis.to / 1000)
      );
    });

    $(`#${this.props.id}`).bind("plothover", (evt, position) => {
      if (position) {
        this.lockCrosshair({
          x: item.datapoint[0],
          y: item.datapoint[1],
        });
      } else {
        this.unlockCrosshair();
      }
    });
  }
}

const mapStateToProps = (state) => ({
  ...state,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      setDateRange,
      receiveJSON,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(TimelineChart);
