import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { setFile } from '../redux/actions';

function AdhocSingle(props) {
  const { actions, file, flamebearer } = props;
  const { setFile } = actions;

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Box>
          <FlameGraphRenderer
            flamebearer={flamebearer}
            uploader={{ file, setFile }}
            viewType="single"
          />
        </Box>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  file: state.root.adhocSingle.file,
  flamebearer: state.root.adhocSingle.flamebearer,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({ setFile }, dispatch),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocSingle);
