import { useState, Dispatch, SetStateAction } from 'react';

interface SharedQueryHookProps {
  searchQuery?: string;
  onQueryChange: Dispatch<SetStateAction<string | undefined>>;
  syncEnabled: string | boolean;
  toggleSync: Dispatch<SetStateAction<boolean | string>>;
}

const useFlamegraphSharedQuery = (): SharedQueryHookProps => {
  const [searchQuery, onQueryChange] = useState<string | undefined>();
  const [syncEnabled, toggleSync] = useState<boolean | string>(false);

  return {
    searchQuery,
    onQueryChange,
    syncEnabled,
    toggleSync,
  };
};

export default useFlamegraphSharedQuery;
