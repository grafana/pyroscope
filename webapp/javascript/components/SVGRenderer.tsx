import React, {useEffect} from 'react';
import {connect} from 'react-redux';

import Label from './Label';
import {fetchData} from '../redux/actions';

class SVGRenderer extends React.Component {

  componentDidMount() {
    this.maybeFetchData();
  }

  componentDidUpdate(prevProps) {
    this.maybeFetchData()
    if (window.init) {
      try {
        window.init();
      } catch(e) {
        console.log(e);
      }
    }
    if (window.unzoom) {
      try {
        window.unzoom();
      } catch(e) {
        console.log(e);
      }
    }
  }

  maybeFetchData(){
    let url = this.renderURL("frontend")
    if(this.lastRequestedURL != url) {
      this.lastRequestedURL = url
      this.props.fetchData(url);
    }
  }

  renderURL(format="svg") {
    let width = document.body.clientWidth - 30;
    let url = `/render?format=${format}&from=${encodeURIComponent(this.props.from)}&until=${encodeURIComponent(this.props.until)}&width=${width}`;
    let nameLabel = this.props.labels.find(x => x.name == "__name__");
    if (nameLabel) {
      url += "&name="+nameLabel.value+"{";
    } else {
      url += "&name=unknown{";
    }
    url += this.props.labels.filter(x => x.name != "__name__").map(x => `${x.name}=${x.value}`).join(",");
    url += "}";
    return url;
  }

  render() {
    return (
      <div className="svg-renderer">
        <div className="samples">{
          (this.props.samples || []).map((x) => <div key={x.ts}>{`${x.ts}-${x.samples}`}</div>)
        }</div>
        <div className="svg-container" dangerouslySetInnerHTML={{__html: this.props.svg}}></div>
        {/* <img className="svg-container" src={`data:image/svg+xml;utf8,${this.props.svg}`} /> */}
        {/* {this.props.from} */}
        {/* {this.props.until} */}
      </div>
    );
  }
}



export default connect(
  (x) => x,
  { fetchData }
)(SVGRenderer);

