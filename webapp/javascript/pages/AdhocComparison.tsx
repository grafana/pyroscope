import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useOldRootSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import classNames from 'classnames';
import { Profile } from '@pyroscope/models';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import FileList from '@webapp/components/FileList';
import FileUploader from '@webapp/components/FileUploader';
import Footer from '@webapp/components/Footer';
import {
  fetchAdhocProfiles,
  fetchAdhocLeftProfile,
  fetchAdhocRightProfile,
  setAdhocLeftFile,
  setAdhocLeftProfile,
  setAdhocRightFile,
  setAdhocRightProfile,
  abortFetchAdhocLeftProfile,
  abortFetchAdhocProfiles,
  abortFetchAdhocRightProfile,
} from '@webapp/redux/actions';
import 'react-tabs/style/react-tabs.css';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';
import ExportData from '../components/ExportData';

function AdhocComparison() {
  const dispatch = useAppDispatch();

  const { left, right } = useOldRootSelector((state) => state.adhocComparison);
  const { left: leftShared, right: rightShared } = useOldRootSelector(
    (state) => state.adhocShared
  );

  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(left.raw);
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(
    right.raw
  );

  useEffect(() => {
    dispatch(fetchAdhocProfiles());
    return () => {
      dispatch(abortFetchAdhocProfiles());
    };
  }, [dispatch]);

  useEffect(() => {
    if (leftShared.profile) {
      dispatch(fetchAdhocLeftProfile(leftShared.profile));
    }
    return () => {
      dispatch(abortFetchAdhocLeftProfile());
    };
  }, [dispatch, leftShared.profile]);

  useEffect(() => {
    if (rightShared.profile) {
      dispatch(fetchAdhocRightProfile(rightShared.profile));
    }
    return () => {
      dispatch(abortFetchAdhocRightProfile());
    };
  }, [dispatch, rightShared.profile]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Upload</Tab>
                <Tab>Pyroscope data</Tab>
              </TabList>
              <TabPanel>
                <FileUploader
                  className={adhocStyles.tabPanel}
                  file={left.file}
                  setFile={(f, flame) => dispatch(setAdhocLeftFile(f, flame))}
                />
              </TabPanel>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={leftShared.profile}
                  setProfile={(p: Profile) => {
                    dispatch(setAdhocLeftProfile(p));
                  }}
                />
              </TabPanel>
            </Tabs>
            {left.isProfileLoading && (
              <div className={classNames('spinner-container')}>
                <Spinner color="rgba(255,255,255,0.6)" size="20px" />
              </div>
            )}
            {!left.isProfileLoading && (
              <FlamegraphRenderer
                flamebearer={left.flamebearer}
                data-testid="flamegraph-renderer-left"
                ExportData={
                  <ExportData
                    flamebearer={left.raw}
                    exportJSON
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComLeftFn}
                  />
                }
              />
            )}
          </Box>
          {/* Right side */}
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Upload</Tab>
                <Tab>Pyroscope data</Tab>
              </TabList>
              <TabPanel>
                <FileUploader
                  className={adhocStyles.tabPanel}
                  file={left.file}
                  setFile={(f, flame) => dispatch(setAdhocRightFile(f, flame))}
                />
              </TabPanel>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={rightShared.profile}
                  setProfile={(p: Profile) => {
                    dispatch(setAdhocRightProfile(p));
                  }}
                />
              </TabPanel>
            </Tabs>
            {right.isProfileLoading && (
              <div className={classNames('spinner-container')}>
                <Spinner color="rgba(255,255,255,0.6)" size="20px" />
              </div>
            )}
            {!right.isProfileLoading && (
              <FlamegraphRenderer
                flamebearer={right.flamebearer}
                data-testid="flamegraph-renderer-right"
                ExportData={
                  <ExportData
                    flamebearer={right.raw}
                    exportJSON
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComRightFn}
                  />
                }
              />
            )}
          </Box>
        </div>
      </div>
      <Footer />
    </div>
  );
}

export default AdhocComparison;
