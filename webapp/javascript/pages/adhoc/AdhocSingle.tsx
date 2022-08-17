import React, { useEffect, useState } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import FileList from '@webapp/components/FileList';
import 'react-tabs/style/react-tabs.css';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import ExportData from '@webapp/components/ExportData';
import {
  uploadFile,
  fetchProfile,
  selectShared,
  fetchAllProfiles,
  selectedSelectedProfileId,
  selectProfile,
} from '@webapp/redux/reducers/adhoc';
import useColorMode from '@webapp/hooks/colorMode.hook';
import FileUploader from './components/FileUploader';
import adhocStyles from './Adhoc.module.scss';

function AdhocSingle() {
  const dispatch = useAppDispatch();
  const { profilesList } = useAppSelector(selectShared);
  const selectedProfileId = useAppSelector(selectedSelectedProfileId('left'));
  const profile = useAppSelector(selectProfile('left'));
  const [tabIndex, setTabIndex] = useState(0);
  useColorMode();

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

  return (
    <div className="main-wrapper">
      <Box>
        <Tabs selectedIndex={tabIndex} onSelect={(index) => setTabIndex(index)}>
          <TabList>
            <Tab>Upload</Tab>
            <Tab>Pyroscope data</Tab>
          </TabList>
          <TabPanel>
            <FileUploader
              className={adhocStyles.tabPanel}
              setFile={async ({ file }) => {
                await dispatch(uploadFile({ file, side: 'left' }));
                setTabIndex(1);
              }}
            />
          </TabPanel>
          <TabPanel>
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
        </Tabs>
        {flame}
      </Box>
    </div>
  );
}

export default AdhocSingle;
