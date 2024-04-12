import React, { useEffect } from 'react';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@pyroscope/ui/Menu';
import Dropdown, { MenuItem } from '@pyroscope/ui/Dropdown';
import { TagsState, ALL_TAGS } from '@pyroscope/redux/reducers/continuous';
import styles from '@pyroscope/pages/TagExplorerView.module.scss';
import { getProfileMetricTitle } from '@pyroscope/pages/TagExplorerView';

export function ExploreHeader({
  appName,
  whereDropdownItems,
  tags,
  selectedTag,
  selectedTagValue,
  handleGroupByTagChange,
  handleGroupByTagValueChange,
}: {
  appName: Maybe<string>;
  whereDropdownItems: string[];
  tags: TagsState;
  selectedTag: string;
  selectedTagValue: string;
  handleGroupByTagChange: (value: string) => void;
  handleGroupByTagValueChange: (value: string) => void;
}) {
  const tagKeys = Object.keys(tags.tags);

  const groupByDropdownItems =
    tagKeys.length > 0 ? tagKeys : ['No tags available'];

  const handleGroupByClick = (e: ClickEvent) => {
    handleGroupByTagChange(e.value);
  };

  const handleGroupByValueClick = (e: ClickEvent) => {
    handleGroupByTagValueChange(e.value);
  };

  useEffect(() => {
    if (tagKeys.length && !selectedTag) {
      const firstGoodTag = tagKeys.find((tag) => !tag.startsWith('__'));
      handleGroupByTagChange(firstGoodTag || tagKeys[0]);
    }
  }, [tagKeys, selectedTag, handleGroupByTagChange]);

  return (
    <div className={styles.header} data-testid="explore-header">
      <span className={styles.title}>{getProfileMetricTitle(appName)}</span>
      <div className={styles.queryGrouppedBy}>
        <span className={styles.selectName}>grouped by</span>
        <Dropdown
          label="select tag"
          value={selectedTag ? `tag: ${selectedTag}` : 'select tag'}
          onItemClick={tagKeys.length > 0 ? handleGroupByClick : undefined}
          menuButtonClassName={
            selectedTag === '' ? styles.notSelectedTagDropdown : undefined
          }
        >
          {groupByDropdownItems.map((tagName) => (
            <MenuItem key={tagName} value={tagName}>
              {tagName}
            </MenuItem>
          ))}
        </Dropdown>
      </div>
      <div className={styles.query}>
        <span className={styles.selectName}>where</span>
        <Dropdown
          label="select where"
          value={`${selectedTag ? `${selectedTag} = ` : selectedTag} ${
            selectedTagValue || ALL_TAGS
          }`}
          onItemClick={handleGroupByValueClick}
          menuButtonClassName={styles.whereSelectButton}
        >
          {/* always show "All" option */}
          {[ALL_TAGS, ...whereDropdownItems].map((tagGroupName) => (
            <MenuItem key={tagGroupName} value={tagGroupName}>
              {tagGroupName}
            </MenuItem>
          ))}
        </Dropdown>
      </div>
    </div>
  );
}
