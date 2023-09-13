import React, { useEffect, useState } from 'react';
import 'react-dom';
import { Maybe } from '@pyroscope/util/fp';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@pyroscope/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
import { Profile } from '@pyroscope/legacy/models';
import FileList from '@pyroscope/components/FileList';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import ExportData from '@pyroscope/components/ExportData';
import {
  fetchAllProfiles,
  fetchProfile,
  selectedSelectedProfileId,
  selectProfile,
  selectShared,
  uploadFile,
} from '@pyroscope/redux/reducers/adhoc';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import { Tabs, Tab, TabPanel } from '@pyroscope/ui/Tabs';
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';
import FileUploader from './components/FileUploader';
import { PageContentWrapper } from '@pyroscope/pages/PageContentWrapper';

function AdhocComparison() {
  const dispatch = useAppDispatch();
  useColorMode();
  const [tabIndexLeft, setTabIndexLeft] = useState(0);
  const [tabIndexRight, setTabIndexRight] = useState(0);

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
      <PageContentWrapper>
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          {/* Left side */}
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
            {leftFlamegraph}
          </Box>
          {/* Right side */}
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
                    uploadFile({
                      file,
                      spyName,
                      units,
                      side: 'right',
                    })
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
            {rightFlamegraph}
          </Box>
        </div>
      </PageContentWrapper>
    </div>
  );
}

export default AdhocComparison;
