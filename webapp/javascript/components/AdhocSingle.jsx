import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { setFile } from '../redux/actions';

function AdhocSingle(props) {
  const { actions, file, flamebearer } = props;
  const { setFile } = actions;

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <FlameGraphRenderer
          flamebearer={flamebearer}
          uploader={{ file, setFile }}
          viewType="single"
        />
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
  file: state.adhocSingle.file,
  flamebearer: state.adhocSingle.flamebearer,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({ setFile }, dispatch),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocSingle);
