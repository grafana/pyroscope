import React, { useMemo, useState } from 'react';

import { Profile } from '@pyroscope/models/src';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src';
import {
  calleesProfile,
  callersProfile,
} from '@pyroscope/flamegraph/src/convert/sandwichViewProfiles';
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
  // to debug
  const [flameType, setFlameType] = useState('callers');
  const { query } = useAppSelector(selectQueries);

  const profile = useMemo(
    () => selectedFunction && calleesProfile(sandwichProfile, selectedFunction),
    [selectedFunction]
  );

  const profile1 = useMemo(
    () => selectedFunction && callersProfile(sandwichProfile, selectedFunction),
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
            {[
              'name',
              'name-2-2',
              'name-3-1',
              'name-5-1',
              'name-5-2',
              'specific-function-name',
            ].map((name) => (
              // {sandwichProfile.flamebearer.names.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
          <button onClick={() => setFlameType('callees')}>
            callees flamegeaph
          </button>
          <button onClick={() => setFlameType('callers')}>
            callers flamegeaph
          </button>
          <br />
          <br />
          {/* will be moved to flamegraph package */}
          <div className={styles.sandwich}>
            {selectedFunction &&
              (flameType === 'callees' ? (
                <>
                  <FlamegraphRenderer
                    showToolbar={false}
                    showCredit={false}
                    profile={profile as Profile}
                  />
                </>
              ) : (
                <>
                  <FlamegraphRenderer
                    showToolbar={false}
                    showCredit={false}
                    profile={profile1 as Profile}
                  />
                </>
              ))}
          </div>
        </Box>
      </div>
    </>
  );
}
