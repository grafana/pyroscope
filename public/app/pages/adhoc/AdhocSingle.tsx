import React, { useEffect, useState } from 'react';
import 'react-dom';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@pyroscope/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
import FileList from '@pyroscope/components/FileList';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import ExportData from '@pyroscope/components/ExportData';
import {
  uploadFile,
  fetchProfile,
  selectShared,
  fetchAllProfiles,
  selectedSelectedProfileId,
  selectProfile,
} from '@pyroscope/redux/reducers/adhoc';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import { Tabs, Tab, TabPanel } from '@pyroscope/ui/Tabs';
import FileUploader from './components/FileUploader';
import adhocStyles from './Adhoc.module.scss';
import { PageContentWrapper } from '@pyroscope/pages/PageContentWrapper';

function AdhocSingle() {
  const dispatch = useAppDispatch();
  const { profilesList } = useAppSelector(selectShared);
  const selectedProfileId = useAppSelector(selectedSelectedProfileId('left'));
  const profile = useAppSelector(selectProfile('left'));
  useColorMode();
  const [currentTab, setCurrentTab] = useState(0);

  useEffect(() => {
    dispatch(fetchAllProfiles());
  }, [dispatch]);

  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    profile.unwrapOr(undefined)
  );

  const flame = (() => {
    if (profile.isNothing) {
      return <></>;
    }

    return (
      <FlamegraphRenderer
        profile={profile.value}
        showCredit={false}
        ExportData={
          <ExportData
            flamebearer={profile.value}
            exportJSON
            exportFlamegraphDotCom
            exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
          />
        }
      />
    );
  })();

  const setFile = async ({
    file,
    spyName,
    units,
  }: {
    file: File;
    spyName?: string;
    units?: string;
  }) => {
    await dispatch(uploadFile({ file, spyName, units, side: 'left' }));
    setCurrentTab(1);
  };

  return (
    <PageContentWrapper>
      <Box>
        <Tabs value={currentTab} onChange={(e, value) => setCurrentTab(value)}>
          <Tab label="Upload" />
          <Tab label="Pyroscope data" />
        </Tabs>
        <TabPanel value={currentTab} index={0}>
          <FileUploader className={adhocStyles.tabPanel} setFile={setFile} />
        </TabPanel>
        <TabPanel value={currentTab} index={1}>
          {profilesList.type === 'loaded' && (
            <FileList
              className={adhocStyles.tabPanel}
              selectedProfileId={selectedProfileId}
              profilesList={profilesList.profilesList}
              onProfileSelected={(id: string) => {
                dispatch(fetchProfile({ id, side: 'left' }));
              }}
            />
          )}
        </TabPanel>
        {flame}
      </Box>
    </PageContentWrapper>
  );
}

export default AdhocSingle;
