import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useOldRootSelector } from '@pyroscope/redux/hooks';
import Box from '@ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { Profile } from '@pyroscope/models';
import classNames from 'classnames';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import FileList from '../components/FileList';
import FileUploader from '../components/FileUploader';
import Footer from '../components/Footer';

import {
  fetchAdhocProfiles,
  fetchAdhocProfile,
  setAdhocFile,
  setAdhocProfile,
  abortFetchAdhocProfiles,
  abortFetchAdhocProfile,
} from '../redux/actions';
import 'react-tabs/style/react-tabs.css';
import adhocStyles from './Adhoc.module.scss';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';

function AdhocSingle() {
  const dispatch = useAppDispatch();

  const { file, profile, flamebearer, isProfileLoading, raw } =
    useOldRootSelector((state) => state.adhocSingle);
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(raw);

  useEffect(() => {
    dispatch(fetchAdhocProfiles());

    return () => {
      dispatch(abortFetchAdhocProfiles());
    };
  }, [dispatch]);

  useEffect(() => {
    if (profile) {
      dispatch(fetchAdhocProfile(profile));
    }
    return () => {
      dispatch(abortFetchAdhocProfile());
    };
  }, [profile, dispatch]);

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
                setFile={(f, flame) => dispatch(setAdhocFile(f, flame))}
              />
            </TabPanel>
            <TabPanel>
              <FileList
                className={adhocStyles.tabPanel}
                profile={profile}
                setProfile={(p: Profile) => dispatch(setAdhocProfile(p))}
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

export default AdhocSingle;
