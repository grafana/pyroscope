import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import classNames from 'classnames';
import FileList from '../components/FileList';
import FileUploader from '../components/FileUploader';
import Footer from '../components/Footer';

import {
  fetchAdhocProfiles,
  fetchAdhocProfile,
  setAdhocFile,
  setAdhocProfile,
} from '../redux/actions';
import 'react-tabs/style/react-tabs.css';
import adhocStyles from './Adhoc.module.scss';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';

function AdhocSingle(props) {
  const { actions, file, profile, flamebearer, isProfileLoading, raw } = props;
  const { setAdhocFile, setAdhocProfile } = actions;
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(raw);

  useEffect(() => {
    actions.fetchAdhocProfiles();
    return actions.abortAdhocFetchProfiles;
  }, []);

  useEffect(() => {
    if (profile) {
      actions.fetchAdhocProfile(profile);
    }
    return actions.abortAdhocFetchProfile;
  }, [profile]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Box>
          <Tabs>
            <TabList>
              <Tab>Upload</Tab>
              <Tab>Pyroscope data</Tab>
            </TabList>
            <TabPanel>
              <FileUploader
                className={adhocStyles.tabPanel}
                file={file}
                setFile={setAdhocFile}
              />
            </TabPanel>
            <TabPanel>
              <FileList
                className={adhocStyles.tabPanel}
                profile={profile}
                setProfile={setAdhocProfile}
              />
            </TabPanel>
          </Tabs>
          {isProfileLoading && (
            <div className={classNames('spinner-container')}>
              <Spinner color="rgba(255,255,255,0.6)" size="20px" />
            </div>
          )}
          {!isProfileLoading && (
            <FlamegraphRenderer
              flamebearer={flamebearer}
              viewType="single"
              display="both"
              ExportData={
                <ExportData
                  flamebearer={raw}
                  exportJSON
                  exportFlamegraphDotCom
                  exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
                />
              }
            />
          )}
        </Box>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  file: state.root.adhocSingle.file,
  raw: state.root.adhocSingle.raw,
  flamebearer: state.root.adhocSingle.flamebearer,
  profile: state.root.adhocSingle.profile,
  isProfileLoading: state.root.adhocSingle.isProfileLoading,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchAdhocProfiles,
      fetchAdhocProfile,
      setAdhocFile,
      setAdhocProfile,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocSingle);
