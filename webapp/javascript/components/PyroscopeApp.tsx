import React from 'react';
import { connect } from "react-redux";
import "react-dom";
import Spinner from "react-svg-spinner";
import DateRangePicker from "./DateRangePicker";
import DownloadButton from './DownloadButton';
import ZoomOutButton from './ZoomOutButton';
import RefreshButton from './RefreshButton';
import SVGRenderer from "./SVGRenderer";
import FlameGraphRenderer from "./FlameGraphRenderer";
import LabelsFilter from "./LabelsFilter";
import Label from "./Label";
import NameSelector from "./NameSelector";
import TimelineChart from "./TimelineChart";

import classNames from "classnames";

import { fetchNames } from "../redux/actions";

class PyroscopeApp extends React.Component {
  constructor(props) {
    super(props);
  }

  componentDidMount = () => {
    this.props.fetchNames();
  }

  renderURL() {
    let width = document.body.clientWidth - 30;
    let url = `/render?from=${encodeURIComponent(this.props.from)}&until=${encodeURIComponent(this.props.until)}&width=${width}`;
    let nameLabel = this.props.labels.find(x => x.name == "__name__");
    if (nameLabel) {
      url += "&name="+nameLabel.value+"{";
    } else {
      url += "&name=unknown{";
    }

    url += this.props.labels.filter(x => x.name != "__name__").map(x => `${x.name}=${x.value}`).join(",");
    url += "}";
    if(this.props.refreshToken){
      url += `&refreshToken=${this.props.refreshToken}`
    }
    return url;
  }

  render() {
    let renderURL = this.renderURL();
    // See docs here: https://github.com/flot/flot/blob/master/API.md
    let flotOptions = {
      margin: {
        top: 0,
        left: 0,
        bottom: 0,
        right: 0,
      },
      selection: {
				mode: "x"
			},
      grid: {
        borderWidth: 1,
        margin:{
          left: 16,
          right: 16,
        }
      },
      yaxis: {
        show: false,
        min: 0,
      },
      points: {
        show: false,
        radius: 0.1
      },
      lines: {
        show: false,
        steps: true,
        lineWidth: 1.0,
      },
      bars: {
        show: true,
        fill: true
      },
      xaxis: {
        mode: "time",
        timezone: "browser",
        reserveSpace: false
      },
    };
    let timeline = this.props.timeline || [];
    timeline = timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1]);
    let flotData = [timeline];
    return (
      <div className="todo-app">
        <div className="navbar">
          <h1 className="logo"></h1>
          <div className="labels">
            <NameSelector/>
            {this.props.labels.filter(x => x.name !== "__name__").map(function(label) {
              return <Label key={label.name} label={label}></Label>;
            })}
          </div>
          {/* <LabelsFilter /> */}
          <div className="navbar-space-filler"></div>
          <div className={
            classNames("navbar-spinner-container", {
              visible: this.props.isSVGLoading
            })
          }>
            <Spinner color="rgba(255,255,255,0.6)" size="20px"/>
          </div>
          <DownloadButton renderURL={renderURL+"&format=svg&download-filename=flamegraph.svg"} />
          &nbsp;
          <RefreshButton/>
          {/* &nbsp; */}
          {/* <ZoomOutButton/> */}
          &nbsp;
          <DateRangePicker />
        </div>
        <TimelineChart id="timeline-chart" options={flotOptions} data={flotData} width="100%" height="100px"/>
        <SVGRenderer renderURL={renderURL+"&format=frontend"}/>
        {/* <FlameGraphRenderer renderURL={renderURL+"&format=json"}/> */}
      </div>
    );
  }
}


export default connect(
  (x) => x,
  { fetchNames }
)(PyroscopeApp);
