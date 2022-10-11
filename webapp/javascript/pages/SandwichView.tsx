import React, { useMemo, useState, useRef } from 'react';

import { Profile } from '@pyroscope/models/src';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src';
import { sandwichViewProfiles } from '@pyroscope/flamegraph/src/convert/sandwichViewProfiles';
import Box from '@webapp/ui/Box';
import PageTitle from '@webapp/components/PageTitle';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import { useAppSelector } from '@webapp/redux/hooks';
import {
  selectQueries,
  selectContinuousState,
} from '@webapp/redux/reducers/continuous';
import sandwichProfile from '../../../cypress/fixtures/simple-golang-app-cpu.json';
import { formatTitle } from './formatTitle';

import styles from './SandwichView.module.scss';

export default function SandwichView() {
  const [selectedFunction, setSelectedFunction] = useState('total');
  const { query } = useAppSelector(selectQueries);
  const { singleView } = useAppSelector(selectContinuousState);

  const profile = useMemo(
    () =>
      selectedFunction &&
      sandwichViewProfiles(sandwichProfile, selectedFunction),
    [selectedFunction]
  );

  const handleSelectChange = (e: any) => {
    const { value } = e.target;
    setSelectedFunction(value);
  };

  return (
    <>
      <PageTitle title={formatTitle('Sandwich View', query)} />
      <div className={styles.sandwichViewContainer}>
        <Toolbar hideTagsBar />
        <Box>
          <h3>Sandwich view</h3>
          <button onClick={() => setSelectedFunction('total')}>reset</button>
          <select value={selectedFunction} onChange={handleSelectChange}>
            {sandwichProfile.flamebearer.names.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
          <div className={styles.sandwich}>
            <div className={styles.half}>
              <h3>callees flamegraph</h3>
              {selectedFunction && (
                <FlamegraphRenderer
                  showToolbar={false}
                  onlyDisplay="flamegraph"
                  showCredit={false}
                  profile={profile as Profile}
                  // ExportData={
                  //   <ExportData
                  //     flamebearer={{
                  //       ...singleView.profile,
                  //       flamebearer: profile.flamebearer,
                  //       metadata: {
                  //         ...profile.flamebearer.metadata,
                  //         ...singleView.profile?.metadata,
                  //       },
                  //     }}
                  //     exportJSON
                  //     exportPprof
                  //   />
                  // }
                />
              )}
            </div>
            <div className={styles.half}></div>
          </div>
        </Box>
      </div>
    </>
  );
}
