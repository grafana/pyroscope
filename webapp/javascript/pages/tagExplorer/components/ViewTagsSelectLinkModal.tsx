import React, { Dispatch, SetStateAction } from 'react';

import { useAppDispatch } from '@webapp/redux/hooks';
import { actions } from '@webapp/redux/reducers/continuous';
import { appendLabelToQuery } from '@webapp/util/appendLabelToQuery';
import { brandQuery } from '@webapp/models/query';
import styles from './ViewTagsSelectLinkModal.module.scss';

const emptyTagsTag = 'No tags available';

interface ViewTagsSelectModalProps {
  whereDropdownItems: string[];
  groupByTag: string;
  appName: string;
  setLinkTagsSelectModalData: Dispatch<
    SetStateAction<{
      baselineTag: string;
      comparisonTag: string;
      linkName: string;
      isModalOpen: boolean;
      shouldRedirect: boolean;
    }>
  >;
  baselineTag: string;
  comparisonTag: string;
  linkName: string;
}

function ViewTagsSelectLink({
  whereDropdownItems,
  groupByTag,
  appName,
  setLinkTagsSelectModalData,
  baselineTag,
  comparisonTag,
  linkName,
}: ViewTagsSelectModalProps) {
  const dispatch = useAppDispatch();

  const handleTagsChange = (
    tag: string,
    key: 'baselineTag' | 'comparisonTag'
  ) => {
    setLinkTagsSelectModalData((currState) => ({ ...currState, [key]: tag }));
  };

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

  const handleCompareClick = () => {
    const { leftQuery, rightQuery } = getNewSearch();

    dispatch(actions.setLeftQuery(brandQuery(leftQuery)));
    dispatch(actions.setRightQuery(brandQuery(rightQuery)));
    setLinkTagsSelectModalData((currState) => ({
      ...currState,
      shouldRedirect: true,
    }));
  };

  return (
    <div className={styles.viewTagsSelectLinkModal}>
      <div className={styles.modalHeader}>Select Tags For {linkName}</div>
      <div className={styles.modalBody}>
        <div className={styles.side}>
          <span className={styles.title}>baseline</span>
          <ul className={styles.tags}>
            {tags.map((tag) => (
              <li
                className={baselineTag === tag ? styles.selected : ''}
                key={tag}
              >
                <input
                  type="button"
                  onClick={
                    tag !== emptyTagsTag
                      ? () => handleTagsChange(tag, 'baselineTag')
                      : undefined
                  }
                  value={tag}
                />
              </li>
            ))}
          </ul>
        </div>
        <div className={styles.side}>
          <span className={styles.title}>comparison</span>
          <ul className={styles.tags}>
            {tags.map((tag) => (
              <li
                className={comparisonTag === tag ? styles.selected : ''}
                key={tag}
              >
                <input
                  type="button"
                  onClick={
                    tag !== emptyTagsTag
                      ? () => handleTagsChange(tag, 'comparisonTag')
                      : undefined
                  }
                  value={tag}
                />
              </li>
            ))}
          </ul>
        </div>
      </div>
      <div className={styles.modalFooter}>
        <input
          type="button"
          className={styles.compareButton}
          onClick={handleCompareClick}
          value="Compare tags"
        />
      </div>
    </div>
  );
}

export default ViewTagsSelectLink;
