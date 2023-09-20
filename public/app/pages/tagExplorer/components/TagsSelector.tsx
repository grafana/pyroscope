import React, { useState } from 'react';
import { Redirect, useLocation } from 'react-router-dom';
import cl from 'classnames';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import { actions, selectQueries } from '@pyroscope/redux/reducers/continuous';
import { appendLabelToQuery } from '@pyroscope/util/query';
import { brandQuery, parse } from '@pyroscope/models/query';
import { PAGES } from '@pyroscope/pages/urls';
import ModalWithToggle from '@pyroscope/ui/Modals/ModalWithToggle';
import styles from './TagsSelector.module.scss';

const emptyTagsTag = 'No tags available';

export interface TagSelectorProps {
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
  const [isModalOpen, setModalOpenStatus] = useState(false);
  const [{ baselineTag, comparisonTag, shouldRedirect }, setSelectedTags] =
    useState(defaultSelectedTags);
  const dispatch = useAppDispatch();
  const { search } = useLocation();
  const { query } = useAppSelector(selectQueries);

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

    const serviceName = parse(brandQuery(query))?.tags?.service_name;
    const tags = serviceName ? `service_name="${serviceName}"` : '';
    const commonQuery = `${appName}{${tags}}`;

    if (baselineTag) {
      newSearch.leftQuery = appendLabelToQuery(
        commonQuery,
        groupByTag,
        baselineTag
      );
    }

    if (comparisonTag) {
      newSearch.rightQuery = appendLabelToQuery(
        commonQuery,
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
          <li
            title={tag}
            className={cl({ [styles.selected]: sideTag === tag })}
            key={tag}
          >
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
    setModalOpenStatus(false);
  };

  const handleOutsideClick = () => {
    setSelectedTags(defaultSelectedTags);
    setModalOpenStatus(false);
  };

  return (
    <ModalWithToggle
      modalClassName={styles.tagsSelector}
      isModalOpen={isModalOpen}
      setModalOpenStatus={setModalOpenStatus}
      customHandleOutsideClick={handleOutsideClick}
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
