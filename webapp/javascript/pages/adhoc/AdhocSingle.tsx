import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { Profile } from '@pyroscope/models/src';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import FileList from '@webapp/components/FileList';

import {
  fetchAdhocProfiles,
  setAdhocProfile,
  abortFetchAdhocProfiles,
} from '@webapp/redux/actions';
import 'react-tabs/style/react-tabs.css';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import ExportData from '@webapp/components/ExportData';
import {
  uploadFile,
  removeFile,
  selectAdhocUpload,
  selectAdhocUploadedFilename,
} from '@webapp/redux/reducers/adhoc';
import FileUploader from './components/FileUploader';
import adhocStyles from './Adhoc.module.scss';

function AdhocSingle() {
  const dispatch = useAppDispatch();
  const state = useAppSelector(selectAdhocUpload({ view: 'singleView' }));
  const filename = useAppSelector(
    selectAdhocUploadedFilename({ view: 'singleView' })
  );

  useEffect(() => {
    dispatch(fetchAdhocProfiles());

    return () => {
      dispatch(abortFetchAdhocProfiles());
    };
  }, [dispatch]);

  // Load the list of profiles after uploading a profile
  useEffect(() => {
    if (state.type === 'loaded') {
      dispatch(fetchAdhocProfiles());
    }
  }, [state, dispatch]);

  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    'profile' in state ? state.profile : undefined
  );

  const flamegraph = (() => {
    switch (state.type) {
      case 'reloading':
      case 'loaded': {
        return (
          <FlamegraphRenderer
            profile={state.profile}
            showCredit={false}
            ExportData={
              <ExportData
                flamebearer={state.profile}
                exportJSON
                exportFlamegraphDotCom
                exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
              />
            }
          />
        );
      }

      default: {
        return <></>;
      }
    }
  })();

  return (
    <div>
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
                removeFile={() => {
                  dispatch(removeFile({ view: 'singleView' }));
                }}
                setFile={(file) => {
                  dispatch(uploadFile({ file, view: 'singleView' }));
                }}
              />
            </TabPanel>
            <TabPanel>
              <FileList
                className={adhocStyles.tabPanel}
                profile={{}}
                setProfile={(p: Profile) => dispatch(setAdhocProfile(p))}
              />
            </TabPanel>
          </Tabs>
          {flamegraph}
        </Box>
      </div>
    </div>
  );
}

export default AdhocSingle;
