import React, { useEffect, useState } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchPyrescopeAppData,
  abortTimelineRequest,
} from '../redux/actions';
import onFileUpload from '../util/onFileUpload';

function AdhocSingle(props) {
  const { actions, renderURL } = props;
  const [flamebearer, setFlamebearer] = useState();

  useEffect(() => {
    return actions.abortTimelineRequest;
  }, [renderURL]);

  const onUpload = (data) => onFileUpload(data, setFlamebearer);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <FlameGraphRenderer
          flamebearer={flamebearer}
          uploader={onUpload}
          viewType="single"
        />
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
  renderURL: buildRenderURL(state),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchPyrescopeAppData,
      fetchNames,
      abortTimelineRequest,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocSingle);
