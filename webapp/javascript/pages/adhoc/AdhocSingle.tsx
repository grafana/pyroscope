import React, { useEffect } from 'react';
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
  selectAdhocUploadedFilename,
  fetchProfile,
  selectShared,
  fetchAllProfiles,
  selectedSelectedProfileId,
  selectProfile,
} from '@webapp/redux/reducers/adhoc';
import FileUploader from './components/FileUploader';
import adhocStyles from './Adhoc.module.scss';

function AdhocSingle() {
  const dispatch = useAppDispatch();
  const filename = useAppSelector(selectAdhocUploadedFilename('left'));
  const { profilesList } = useAppSelector(selectShared);
  const selectedProfileId = useAppSelector(selectedSelectedProfileId('left'));
  const profile = useAppSelector(selectProfile('left'));

  useEffect(() => {
    dispatch(fetchAllProfiles());

    // TODO(eh-am): abort
    //    return () => {
    //      dispatch(abortFetchAdhocProfiles());
    //    };
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
        <Tabs>
          <TabList>
            <Tab>Upload</Tab>
            <Tab>Pyroscope data</Tab>
          </TabList>
          <TabPanel>
            <FileUploader
              className={adhocStyles.tabPanel}
              filename={filename}
              setFile={(file) => {
                dispatch(
                  uploadFile({ file, view: 'singleView', side: 'left' })
                );
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
