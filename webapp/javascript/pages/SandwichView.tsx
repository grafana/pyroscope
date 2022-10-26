import React, { useState } from 'react';

// import { Profile } from '@pyroscope/models/src';
// import { FlamegraphRenderer } from '@pyroscope/flamegraph/src';
// import {
//   calleesProfile,
//   callersProfile,
// } from '@pyroscope/flamegraph/src/convert/sandwichViewProfiles';
import Box from '@webapp/ui/Box';
import PageTitle from '@webapp/components/PageTitle';
import Toolbar from '@webapp/components/Toolbar';
import { useAppSelector } from '@webapp/redux/hooks';
import { selectQueries } from '@webapp/redux/reducers/continuous';
// import sandwichProfile from '../../../cypress/fixtures/simple-golang-app-cpu.json';
import { formatTitle } from './formatTitle';

import styles from './SandwichView.module.scss';

// page will be removed after implementing sandwich view inside flamegraph package
export default function SandwichView() {
  const [selectedFunction, setSelectedFunction] = useState('total');
  // to debug
  const { query } = useAppSelector(selectQueries);

  // const profile = useMemo(
  //   () =>
  //     selectedFunction &&
  //     calleesProfile(sandwichProfile as ShamefulAny, selectedFunction),
  //   [selectedFunction]
  // );

  // const profile1 = useMemo(
  //   () =>
  //     selectedFunction &&
  //     callersProfile(sandwichProfile as ShamefulAny, selectedFunction),
  //   [selectedFunction]
  // );

  const handleSelectChange = (e: ShamefulAny) => {
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
            {/* {sandwichProfile.flamebearer.names.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))} */}
          </select>
          <br />
          <br />
          {/* will be moved to flamegraph package */}
          <div className={styles.sandwich}>
            {selectedFunction && (
              <>
                {/* <FlamegraphRenderer
                  onlyDisplay="flamegraph"
                  showToolbar={false}
                  showCredit={false}
                  profile={profile as ShamefulAny}
                />
                <FlamegraphRenderer
                  onlyDisplay="flamegraph"
                  showToolbar={false}
                  showCredit={false}
                  profile={profile1 as ShamefulAny}
                /> */}
              </>
            )}
          </div>
        </Box>
      </div>
    </>
  );
}
