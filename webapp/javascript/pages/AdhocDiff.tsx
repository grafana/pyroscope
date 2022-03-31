import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useOldRootSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import classNames from 'classnames';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { Profile } from '@pyroscope/models';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import FileList from '@webapp/components/FileList';
import Footer from '@webapp/components/Footer';
import {
  fetchAdhocProfiles,
  fetchAdhocProfileDiff,
  setAdhocLeftProfile,
  setAdhocRightProfile,
  abortFetchAdhocProfileDiff,
  abortFetchAdhocProfiles,
} from '@webapp/redux/actions';
import 'react-tabs/style/react-tabs.css';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import ExportData from '@webapp/components/ExportData';
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';

function AdhocDiff() {
  const dispatch = useAppDispatch();
  const { flamebearer, isProfileLoading, raw } = useOldRootSelector(
    (state) => state.adhocComparisonDiff
  );
  const { left: leftShared, right: rightShared } = useOldRootSelector(
    (state) => state.adhocShared
  );
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(raw);

  useEffect(() => {
    dispatch(fetchAdhocProfiles());
    return () => {
      return dispatch(abortFetchAdhocProfiles());
    };
  }, [dispatch]);

  useEffect(() => {
    if (leftShared.profile && rightShared.profile) {
      dispatch(fetchAdhocProfileDiff(leftShared.profile, rightShared.profile));
    }
    return () => {
      dispatch(abortFetchAdhocProfileDiff());
    };
  }, [dispatch, leftShared.profile, rightShared.profile]);

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
                <Tab>Pyroscope data</Tab>
                <Tab disabled>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={leftShared.profile}
                  setProfile={(p: Profile) => dispatch(setAdhocLeftProfile(p))}
                />
              </TabPanel>
              <TabPanel />
            </Tabs>
          </Box>
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Pyroscope data</Tab>
                <Tab disabled>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={rightShared.profile}
                  setProfile={(p: Profile) => dispatch(setAdhocRightProfile(p))}
                />
              </TabPanel>
              <TabPanel />
            </Tabs>
          </Box>
        </div>
        <Box>
          {isProfileLoading && (
            <div className={classNames('spinner-container')}>
              <Spinner color="rgba(255,255,255,0.6)" size="20px" />
            </div>
          )}
          {!isProfileLoading && (
            <FlamegraphRenderer
              flamebearer={flamebearer}
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

export default AdhocDiff;
