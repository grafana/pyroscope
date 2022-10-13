import React, { useMemo, useState } from 'react';

import { Profile } from '@pyroscope/models/src';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src';
import { sandwichViewProfiles } from '@pyroscope/flamegraph/src/convert/sandwichViewProfiles';
import Box from '@webapp/ui/Box';
import PageTitle from '@webapp/components/PageTitle';
import Toolbar from '@webapp/components/Toolbar';
import { useAppSelector } from '@webapp/redux/hooks';
import { selectQueries } from '@webapp/redux/reducers/continuous';
import sandwichProfile from '../../../cypress/fixtures/simple-golang-app-cpu.json';
import { formatTitle } from './formatTitle';

import styles from './SandwichView.module.scss';

export default function SandwichView() {
  const [selectedFunction, setSelectedFunction] = useState('name');
  const { query } = useAppSelector(selectQueries);

  const [profile] = useMemo(
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
          <button onClick={() => setSelectedFunction('name')}>reset</button>
          <select value={selectedFunction} onChange={handleSelectChange}>
            {/* {sandwichProfile.flamebearer.names.map((name) => ( */}
            {/* array on mocked tree function names */}
            {[
              'name',
              'specific-function-name',
              'name-3-2',
              'name-2-2',
              'name-3-1',
              'name-5-1',
              'name-5-2',
            ].map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
          <br />
          mocked tree
          <br />
          {/* will be moved to flamegraph package */}
          <div className={styles.sandwich}>
            {selectedFunction && (
              <FlamegraphRenderer
                showToolbar={false}
                showCredit={false}
                profile={profile as Profile}
              />
            )}
          </div>
        </Box>
      </div>
    </>
  );
}
