import React, { useEffect } from 'react';
import 'react-dom';

import {
  useAppDispatch,
  useAppSelector,
  useOldRootSelector,
} from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { Profile } from '@pyroscope/models/src';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import FileList from '@webapp/components/FileList';
import {
  fetchAdhocProfiles,
  setAdhocLeftProfile,
  setAdhocRightProfile,
  abortFetchAdhocProfiles,
} from '@webapp/redux/actions';
import 'react-tabs/style/react-tabs.css';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import ExportData from '@webapp/components/ExportData';
import {
  removeFile,
  selectAdhocUpload,
  uploadFile,
} from '@webapp/redux/reducers/adhoc';
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';
import FileUploader from './components/FileUploader';

function AdhocComparison() {
  const dispatch = useAppDispatch();
  const left = useAppSelector(
    selectAdhocUpload({ view: 'comparisonView', side: 'left' })
  );
  const right = useAppSelector(
    selectAdhocUpload({ view: 'comparisonView', side: 'right' })
  );

  const { left: leftShared, right: rightShared } = useOldRootSelector(
    (state) => state.adhocShared
  );

  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(
    (left as any).raw
  );
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(
    (right as any).profile
  );

  useEffect(() => {
    dispatch(fetchAdhocProfiles());
    return () => {
      dispatch(abortFetchAdhocProfiles());
    };
  }, [dispatch]);

  const flamegraph = (side: 'left' | 'right') => {
    const f = side === 'left' ? left : right;

    switch (f.type) {
      case 'reloading':
      case 'loaded': {
        return (
          <FlamegraphRenderer
            profile={f.profile}
            showCredit={false}
            panesOrientation="vertical"
            ExportData={
              <ExportData
                flamebearer={f.profile}
                exportJSON
                exportFlamegraphDotCom
                exportFlamegraphDotComFn={
                  side === 'left'
                    ? exportToFlamegraphDotComLeftFn
                    : exportToFlamegraphDotComRightFn
                }
              />
            }
          />
        );
      }

      default: {
        return <></>;
      }
    }
  };

  const leftFlamegraph = flamegraph('left');
  const rightFlamegraph = flamegraph('right');

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
                  filename={'fileName' in left ? left.fileName : ''}
                  className={adhocStyles.tabPanel}
                  removeFile={() => {
                    dispatch(
                      removeFile({ view: 'comparisonView', side: 'left' })
                    );
                  }}
                  setFile={(file) => {
                    dispatch(
                      uploadFile({ file, view: 'comparisonView', side: 'left' })
                    );
                  }}
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
                  filename={'fileName' in right ? right.fileName : ''}
                  removeFile={() => {
                    dispatch(
                      removeFile({ view: 'comparisonView', side: 'right' })
                    );
                  }}
                  setFile={(file) => {
                    dispatch(
                      uploadFile({
                        file,
                        view: 'comparisonView',
                        side: 'right',
                      })
                    );
                  }}
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
            {rightFlamegraph}
          </Box>
        </div>
      </div>
    </div>
  );
}

export default AdhocComparison;
