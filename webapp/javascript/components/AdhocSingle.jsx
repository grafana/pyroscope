import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import classNames from 'classnames';
import FileList from './FileList';
import FileUploader from './FileUploader';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';

import {
  fetchProfiles,
  fetchProfile,
  setFile,
  setProfile,
} from '../redux/actions';
import 'react-tabs/style/react-tabs.css';

function AdhocSingle(props) {
  const { actions, file, profile, flamebearer, isProfileLoading } = props;
  const { setFile, setProfile } = actions;

  useEffect(() => {
    actions.fetchProfiles();
    return actions.abortFetchProfiles;
  }, []);

  useEffect(() => {
    if (profile) {
      actions.fetchProfile(profile);
    }
    return actions.abortFetchProfile;
  }, [profile]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Box>
          <Tabs>
            <TabList>
              <Tab>Pyroscope data</Tab>
              <Tab>Upload</Tab>
            </TabList>
            <TabPanel>
              <FileList profile={profile} setProfile={setProfile} />
            </TabPanel>
            <TabPanel>
              <FileUploader file={file} setFile={setFile} />
            </TabPanel>
          </Tabs>
          <div
            className={classNames('spinner-container', {
              visible: isProfileLoading,
            })}
          >
            <Spinner color="rgba(255,255,255,0.6)" size="20px" />
          </div>
          <FlameGraphRenderer
            flamebearer={flamebearer}
            viewType="single"
            display="both"
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
  profile: state.root.adhocSingle.profile,
  isProfileLoading: state.root.adhocSingle.isProfileLoading,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchProfiles,
      fetchProfile,
      setFile,
      setProfile,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocSingle);
