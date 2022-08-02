import React, { useEffect } from 'react';
import 'react-dom';

import { Maybe } from '@webapp/util/fp';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { Profile } from '@pyroscope/models/src';
import FileList from '@webapp/components/FileList';
import 'react-tabs/style/react-tabs.css';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import ExportData from '@webapp/components/ExportData';
import {
  fetchAllProfiles,
  fetchProfile,
  selectAdhocUploadedFilename,
  selectedSelectedProfileId,
  selectProfile,
  selectShared,
  uploadFile,
} from '@webapp/redux/reducers/adhoc';
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';
import FileUploader from './components/FileUploader';

function AdhocComparison() {
  const dispatch = useAppDispatch();

  const leftFilename = useAppSelector(selectAdhocUploadedFilename('left'));
  const rightFilename = useAppSelector(selectAdhocUploadedFilename('right'));

  const leftProfile = useAppSelector(selectProfile('left'));
  const rightProfile = useAppSelector(selectProfile('right'));

  const selectedProfileIdLeft = useAppSelector(
    selectedSelectedProfileId('left')
  );
  const selectedProfileIdRight = useAppSelector(
    selectedSelectedProfileId('right')
  );

  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(
    leftProfile.unwrapOr(undefined)
  );
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(
    rightProfile.unwrapOr(undefined)
  );

  const { profilesList } = useAppSelector(selectShared);

  useEffect(() => {
    dispatch(fetchAllProfiles());
  }, [dispatch]);

  const flamegraph = (
    profile: Maybe<Profile>,
    exportToFn: typeof exportToFlamegraphDotComLeftFn
  ) => {
    if (profile.isNothing) {
      return <></>;
    }

    return (
      <FlamegraphRenderer
        profile={profile.value}
        showCredit={false}
        panesOrientation="vertical"
        ExportData={
          <ExportData
            flamebearer={profile.value}
            exportJSON
            exportFlamegraphDotCom
            exportFlamegraphDotComFn={exportToFn}
          />
        }
      />
    );
  };

  const leftFlamegraph = flamegraph(
    leftProfile,
    exportToFlamegraphDotComLeftFn
  );
  const rightFlamegraph = flamegraph(
    rightProfile,
    exportToFlamegraphDotComRightFn
  );

  return (
    <div>
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
                  filename={leftFilename}
                  className={adhocStyles.tabPanel}
                  setFile={(file) => {
                    dispatch(uploadFile({ file, side: 'left' }));
                  }}
                />
              </TabPanel>
              <TabPanel>
                {profilesList.type === 'loaded' && (
                  <FileList
                    className={adhocStyles.tabPanel}
                    profilesList={profilesList.profilesList}
                    selectedProfileId={selectedProfileIdLeft}
                    onProfileSelected={(id: string) => {
                      dispatch(fetchProfile({ id, side: 'left' }));
                    }}
                  />
                )}
              </TabPanel>
            </Tabs>
            {leftFlamegraph}
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
                  filename={rightFilename}
                  setFile={(file) => {
                    dispatch(
                      uploadFile({
                        file,
                        side: 'right',
                      })
                    );
                  }}
                />
              </TabPanel>
              <TabPanel>
                {profilesList.type === 'loaded' && (
                  <FileList
                    className={adhocStyles.tabPanel}
                    profilesList={profilesList.profilesList}
                    selectedProfileId={selectedProfileIdRight}
                    onProfileSelected={(id: string) => {
                      dispatch(fetchProfile({ id, side: 'right' }));
                    }}
                  />
                )}
              </TabPanel>
            </Tabs>
            {rightFlamegraph}
          </Box>
        </div>
      </div>
    </div>
  );
}

export default AdhocComparison;
