import React, { useState } from 'react';
import { Redirect, useLocation } from 'react-router-dom';

import { useAppDispatch } from '@webapp/redux/hooks';
import { actions } from '@webapp/redux/reducers/continuous';
import { appendLabelToQuery } from '@webapp/util/query';
import { brandQuery } from '@webapp/models/query';
import { PAGES } from '@webapp/pages/constants';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';
import styles from './TagsSelector.module.scss';

const emptyTagsTag = 'No tags available';

interface TagSelectorProps {
  appName: string;
  groupByTag: string;
  linkName: string;
  whereDropdownItems: string[];
}

const defaultSelectedTags = {
  baselineTag: '',
  comparisonTag: '',
  shouldRedirect: false,
};

function TagSelector({
  appName,
  groupByTag,
  linkName,
  whereDropdownItems,
}: TagSelectorProps) {
  const [{ baselineTag, comparisonTag, shouldRedirect }, setSelectedTags] =
    useState(defaultSelectedTags);
  const dispatch = useAppDispatch();
  const { search } = useLocation();

  if (shouldRedirect) {
    return (
      <Redirect
        to={
          (linkName === 'Diff'
            ? PAGES.COMPARISON_DIFF_VIEW
            : PAGES.COMPARISON_VIEW) + search
        }
      />
    );
  }

  const tags =
    whereDropdownItems.length > 0 ? whereDropdownItems : [emptyTagsTag];

  const getNewSearch = () => {
    const newSearch = { leftQuery: '', rightQuery: '' };

    if (baselineTag) {
      newSearch.leftQuery = appendLabelToQuery(
        `${appName}{}`,
        groupByTag,
        baselineTag
      );
    }

    if (comparisonTag) {
      newSearch.rightQuery = appendLabelToQuery(
        `${appName}{}`,
        groupByTag,
        comparisonTag
      );
    }

    return newSearch;
  };

  const renderSide = (
    side: 'left' | 'right',
    sideTag: string,
    tagsSideName: string
  ) => (
    <>
      <span className={styles.title}>
        {side === 'left' ? 'baseline' : 'comparison'}
      </span>
      <ul className={styles.tags}>
        {tags.map((tag) => (
          <li className={sideTag === tag ? styles.selected : ''} key={tag}>
            <input
              type="button"
              onClick={
                tag !== emptyTagsTag
                  ? () =>
                      setSelectedTags((currState) => ({
                        ...currState,
                        [tagsSideName]: tag,
                      }))
                  : undefined
              }
              value={tag}
            />
          </li>
        ))}
      </ul>
    </>
  );

  const handleCompareClick = () => {
    const { leftQuery, rightQuery } = getNewSearch();

    dispatch(actions.setLeftQuery(brandQuery(leftQuery)));
    dispatch(actions.setRightQuery(brandQuery(rightQuery)));
    setSelectedTags((currState) => ({ ...currState, shouldRedirect: true }));
  };

  const handleOutClick = () => {
    setSelectedTags(defaultSelectedTags);
  };

  return (
    <ModalWithToggle
      handleOutClick={handleOutClick}
      toggleText={linkName}
      headerEl={`Select Tags For ${linkName}`}
      leftSideEl={renderSide('left', baselineTag, 'baselineTag')}
      rightSideEl={renderSide('right', comparisonTag, 'comparisonTag')}
      footerEl={
        <input
          type="button"
          className={styles.compareButton}
          onClick={handleCompareClick}
          value="Compare tags"
        />
      }
    />
  );
}

export default TagSelector;
