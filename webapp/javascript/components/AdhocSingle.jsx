import React, { useEffect, useRef, useState } from 'react';
import { connect } from 'react-redux';
import Dropzone from 'react-dropzone';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Header from './Header';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchPyrescopeAppData,
  abortTimelineRequest,
} from '../redux/actions';
import FileUploader from './FileUploader';
import { deltaDiffWrapper } from '../util/flamebearer';

function AdhocSingle(props) {
  const { actions, renderURL, single } = props;
  const prevPropsRef = useRef();
  const [flamebearer, setFlamebearer] = useState();

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      actions.fetchPyrescopeAppData(renderURL);
    }

    return actions.abortTimelineRequest;
  }, [renderURL]);

  const onFileUpload = (data) => {
    if (!data) {
      setFlamebearer(null);
      return;
    }

    const { flamebearer } = data;

    const calculatedLevels = deltaDiffWrapper(
      flamebearer.format,
      flamebearer.levels
    );

    flamebearer.levels = calculatedLevels;
    setFlamebearer(flamebearer);
  };

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <FileUploader onUpload={onFileUpload} />
        <FlameGraphRenderer flamebearer={flamebearer} viewType="single" />
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  renderURL: buildRenderURL(state.root),
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
