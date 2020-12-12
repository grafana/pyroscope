import React from 'react';
import { connect } from "react-redux";
import { setMaxNodes } from "../redux/actions";

class MaxNodesSelector extends React.Component {
  constructor(props) {
    super(props);
  }

  select = (event) =>{
    this.props.setMaxNodes(parseInt(event.target.value));
  }

  render() {
    let options = [
      1024,
      2048,
      4096,
      8192,
      16384,
      32768,
      65536,
    ];
    let selected = this.props.maxNodes;
    return <span>
      Max Nodes:&nbsp;
      <select className="max-nodes-select" value={selected} onChange={this.select}>
        {options.map(function(name) {
          return <option
            key={name}
            value={name}
          >{name}</option>;
        })}
      </select>
    </span>
  }
}

export default connect(
  (x) => x,
  { setMaxNodes }
)(MaxNodesSelector);
