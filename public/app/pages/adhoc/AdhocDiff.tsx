import React, { useEffect, useState } from 'react';
import 'react-dom';
import { Maybe } from '@phlare/util/fp';
import { useAppDispatch, useAppSelector } from '@phlare/redux/hooks';
import Box from '@phlare/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { Profile } from '@pyroscope/models/src';
import FileList from '@phlare/components/FileList';
import useExportToFlamegraphDotCom from '@phlare/components/exportToFlamegraphDotCom.hook';
import ExportData from '@phlare/components/ExportData';
import {
  fetchAllProfiles,
  fetchDiffProfile,
  fetchProfile,
  selectedSelectedProfileId,
  selectProfileId,
  selectShared,
  selectDiffProfile,
  uploadFile,
} from '@phlare/redux/reducers/adhoc';
import useColorMode from '@phlare/hooks/colorMode.hook';
import { Tabs, Tab, TabPanel } from '@phlare/ui/Tabs';
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';
import FileUploader from './components/FileUploader';

function AdhocDiff() {
  const dispatch = useAppDispatch();
  const leftProfileId = useAppSelector(selectProfileId('left'));
  const rightProfileId = useAppSelector(selectProfileId('right'));
  useColorMode();
  const selectedProfileIdLeft = useAppSelector(
    selectedSelectedProfileId('left')
  );
  const selectedProfileIdRight = useAppSelector(
    selectedSelectedProfileId('right')
  );
  const { profilesList } = useAppSelector(selectShared);
  const diffProfile = useAppSelector(selectDiffProfile);
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    diffProfile.unwrapOr(undefined)
  );
  const [tabIndexLeft, setTabIndexLeft] = useState(0);
  const [tabIndexRight, setTabIndexRight] = useState(0);

  useEffect(() => {
    dispatch(fetchAllProfiles());
  }, [dispatch]);

  const unwrappedLeftProfileId = leftProfileId.unwrapOr(undefined);
  const unwrappedRightProfileId = rightProfileId.unwrapOr(undefined);

  useEffect(() => {
    if (unwrappedLeftProfileId && unwrappedRightProfileId) {
      dispatch(
        fetchDiffProfile({
          leftId: unwrappedLeftProfileId,
          rightId: unwrappedRightProfileId,
        })
      );
    }
  }, [
    dispatch,
    unwrappedLeftProfileId,
    unwrappedRightProfileId
  ]);

  const flamegraph = (
    profile: Maybe<Profile>,
    exportToFn: ReturnType<typeof useExportToFlamegraphDotCom>
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

  return (
    <div>
      <div className="main-wrapper">
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs
              value={tabIndexLeft}
              onChange={(e, value) => setTabIndexLeft(value)}
            >
              <Tab label="Upload" />
              <Tab label="Pyroscope data" />
            </Tabs>
            <TabPanel value={tabIndexLeft} index={0}>
              <FileUploader
                className={adhocStyles.tabPanel}
                setFile={async ({ file, spyName, units }) => {
                  await dispatch(
                    uploadFile({ file, spyName, units, side: 'left' })
                  );
                  setTabIndexLeft(1);
                }}
              />
            </TabPanel>
            <TabPanel value={tabIndexLeft} index={1}>
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
          </Box>
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs
              value={tabIndexRight}
              onChange={(e, value) => setTabIndexRight(value)}
            >
              <Tab label="Upload" />
              <Tab label="Pyroscope data" />
            </Tabs>
            <TabPanel value={tabIndexRight} index={0}>
              <FileUploader
                className={adhocStyles.tabPanel}
                setFile={async ({ file, spyName, units }) => {
                  await dispatch(
                    uploadFile({ file, spyName, units, side: 'right' })
                  );
                  setTabIndexRight(1);
                }}
              />
            </TabPanel>
            <TabPanel value={tabIndexRight} index={1}>
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
          </Box>
        </div>
        <Box>{flamegraph(diffProfile, exportToFlamegraphDotComFn)}</Box>
      </div>
    </div>
  );
}

export default AdhocDiff;
