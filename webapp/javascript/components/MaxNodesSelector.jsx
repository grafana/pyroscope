import React from "react";
import { connect } from "react-redux";
import { setMaxNodes } from "../redux/actions";

class MaxNodesSelector extends React.Component {
  select = (event) => {
    // eslint-disable-next-line radix, react/destructuring-assignment
    this.props.setMaxNodes(parseInt(event.target.value));
  };

  render() {
    const options = [1024, 2048, 4096, 8192, 16384, 32768, 65536];
    // eslint-disable-next-line react/destructuring-assignment
    const selected = this.props.maxNodes;
    return (
      <span>
        Max Nodes:&nbsp;
        <select
          className="max-nodes-select"
          value={selected}
          onChange={this.select}
        >
          {options.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </span>
    );
  }
}

export default connect((x) => x, { setMaxNodes })(MaxNodesSelector);
