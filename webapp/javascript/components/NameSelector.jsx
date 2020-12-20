import React from 'react';
import { connect } from "react-redux";
import { addLabel, receiveJSON} from "../redux/actions";
import { bindActionCreators } from "redux";
import { buildRenderURL, fetchJSON } from '../util/update_requests';


class NameSelector extends React.Component {
  constructor(props) {
    super(props);

    this.fetchJSON = fetchJSON.bind(this);
    this.buildRenderURL = buildRenderURL.bind(this);
  }

  selectName = (event) =>{
    this.props.actions.addLabel("__name__", event.target.value)
    .then(() => {
      let renderURL = this.buildRenderURL();
      this.fetchJSON(renderURL);
    })
  }

  render() {
    let names = this.props.names || [];
    let selectedName = this.props.labels.filter(x => x.name == "__name__")[0];
    selectedName = selectedName ? selectedName.value : "none";
    return <span>
      Metric:&nbsp;
      <select className="label-select" value={selectedName} onChange={this.selectName}>
        <option
          disabled
          key="Select an app..."
          value="Select an app..."
        >Select an app...</option>
        {names.map(function(name) {
          return <option
            key={name}
            value={name}
          >{name}</option>;
        })}
      </select>
    </span>
  }
}

const mapStateToProps = state => ({
  ...state,
});

const mapDispatchToProps = dispatch => ({
  actions: bindActionCreators(
    {
      receiveJSON,
      addLabel,
    },
    dispatch,
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps,
)(NameSelector);
