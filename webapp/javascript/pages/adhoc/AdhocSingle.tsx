import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import FileList from '@webapp/components/FileList';

import {
  fetchAdhocProfiles,
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
  fetchProfile,
  selectShared,
} from '@webapp/redux/reducers/adhoc';
import { Profile } from '@pyroscope/models/src';
import FileUploader from './components/FileUploader';
import adhocStyles from './Adhoc.module.scss';

function AdhocSingle() {
  const dispatch = useAppDispatch();
  const state = useAppSelector(selectAdhocUpload({ view: 'singleView' }));
  const filename = useAppSelector(
    selectAdhocUploadedFilename({ view: 'singleView' })
  );
  const { left } = useAppSelector(selectShared);

  useEffect(() => {
    dispatch(fetchAdhocProfiles());

    return () => {
      dispatch(abortFetchAdhocProfiles());
    };
  }, [dispatch]);

  // TODO(eh-am): don't use a hook
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    'profile' in state ? state.profile : undefined
  );

  const flame = (profile: Profile) => {
    return (
      <FlamegraphRenderer
        profile={profile}
        showCredit={false}
        ExportData={
          <ExportData
            flamebearer={profile}
            exportJSON
            exportFlamegraphDotCom
            exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
          />
        }
      />
    );
  };

  const decide = (() => {
    if (left.type === 'loaded') {
      return flame(left.profile);
    }

    //    switch (state.type) {
    //      case 'reloading':
    //      case 'loaded': {
    //        return flame(state.profile);
    //      }
    //
    //      default: {
    //        return <></>;
    //      }
    //    }
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
              setProfile={(id: string) =>
                dispatch(fetchProfile({ id, side: 'left' }))
              }
            />
          </TabPanel>
        </Tabs>
        {decide}
      </Box>
    </div>
  );
}

export default AdhocSingle;
