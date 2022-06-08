import React, { useState, useEffect, useRef } from 'react';
import Button from '@webapp/ui/Button';
import 'react-dom';
import { useWindowWidth } from '@react-hook/window-size';
import { TagsState } from '@webapp/redux/reducers/continuous';
import Dropdown, {
  SubMenu,
  MenuItem,
  FocusableItem,
  MenuGroup,
} from '@webapp/ui/Dropdown';
import TextareaAutosize from 'react-textarea-autosize';
import { Prism } from '@webapp/util/prism';
import { Query, brandQuery } from '@webapp/models/query';
import Input from '@webapp/ui/Input';
import styles from './TagsBar.module.css';

interface TagsBarProps {
  onSetQuery: (q: Query) => void;
  onSelectedLabel: (label: string, query: Query) => void;
  query: Query;
  tags: TagsState;
}

function TagsBar({ onSetQuery, query, tags, onSelectedLabel }: TagsBarProps) {
  const CustomDropdown = (() => {
    const noTagsAvailable = (
      <Dropdown label="Select Tag">
        <MenuItem>No tags available</MenuItem>
      </Dropdown>
    );

    if (tags?.type === 'loaded' && Object.keys(tags?.tags || {}).length <= 0) {
      return noTagsAvailable;
    }

    switch (tags.type) {
      case 'loading': {
        return (
          <Dropdown label="Select Tag">
            <MenuItem>Loading</MenuItem>
          </Dropdown>
        );
      }

      case 'loaded': {
        return (
          <LabelsSubmenu
            query={query}
            labels={tags.tags}
            onSelectedLabel={(label) => {
              // if nothing changed there's no point bubbling up
              if (!tags.tags[label] || tags.tags[label].type !== 'loaded') {
                onSelectedLabel(label, query);
              }
            }}
            onSelectedLabelValue={(label, labelValue) => {
              const newQuery = appendLabelToQuery(query, label, labelValue);
              onSetQuery(brandQuery(newQuery));
            }}
          />
        );
      }

      default:
      case 'failed': {
        // There's no need to show anything
        // Since it's likely that a toast already showed something went wrong
        return noTagsAvailable;
      }
    }
  })();

  return (
    <div className="tags-bar _rc-menu-container--theme-dark">
      {CustomDropdown}
      <QueryInput
        initialQuery={query}
        onSubmit={(q) => {
          onSetQuery(brandQuery(q));
        }}
      />
    </div>
  );
}

interface QueryInputProps {
  initialQuery: Query;
  onSubmit: (query: string) => void;
}

function QueryInput({ initialQuery, onSubmit }: QueryInputProps) {
  const windowWidth = useWindowWidth();
  const [query, setQuery] = useState(initialQuery);
  const codeRef = useRef<HTMLElement>(null);
  const textareaRef = useRef<any>(null);
  const [textAreaSize, setTextAreaSize] = useState({ width: 0, height: 0 });

  // If query updated upstream, most likely the application changed
  // So we prefer to use it
  useEffect(() => {
    setQuery(initialQuery);
  }, [initialQuery]);

  useEffect(() => {
    if (Prism && codeRef.current) {
      Prism.highlightElement(codeRef.current);
    }
  }, [query]);

  useEffect(() => {
    setTextAreaSize({
      width: textareaRef?.current?.['offsetWidth'] || 0,
      height: textareaRef?.current?.['offsetHeight'] || 0,
    });
  }, [query, windowWidth, onSubmit]);

  const onFormSubmit = (
    e:
      | React.FormEvent<HTMLFormElement>
      | React.KeyboardEvent<HTMLTextAreaElement>
  ) => {
    e.preventDefault();
    onSubmit(query);
  };

  const handleTextareaKeyDown = (
    e: React.KeyboardEvent<HTMLTextAreaElement>
  ) => {
    if ((e.keyCode === 10 || e.keyCode === 13) && e.ctrlKey) {
      onFormSubmit(e);
    }
  };

  return (
    <form className="tags-query" onSubmit={onFormSubmit}>
      <pre className="tags-highlighted language-promql" aria-hidden="true">
        <code
          className="language-promql"
          id="highlighting-content"
          ref={codeRef}
          style={textAreaSize}
        >
          {query}
        </code>
      </pre>
      <TextareaAutosize
        className="tags-input"
        ref={textareaRef}
        value={query}
        spellCheck="false"
        onChange={(e) => setQuery(brandQuery(e.target.value))}
        onKeyDown={handleTextareaKeyDown}
      />
      <Button
        type="submit"
        kind="secondary"
        grouped
        className={styles.executeButton}
      >
        Execute
      </Button>
    </form>
  );
}

interface LabelsSubmenuProps {
  query: Query;
  labels: TagsState['tags'];
  onSelectedLabel: (tag: string) => void;
  onSelectedLabelValue: (label: string, labelValue: string) => void;
}

function LabelsSubmenu({
  query,
  labels: tags,
  onSelectedLabel,
  onSelectedLabelValue,
}: LabelsSubmenuProps) {
  // TODO: type this properly
  const [filter, setFilter] = useState<Record<string, string>>({});

  const GetTagValues = (labelName: string, t: typeof tags[1]) => {
    const { type } = t;
    switch (type) {
      case 'loading': {
        return <MenuItem>Loading</MenuItem>;
      }

      case 'loaded': {
        const { values } = t;

        return (
          values
            // filter only the ones that match the filter
            .filter((n) => {
              const f = filter[labelName]
                ? filter[labelName].trim().toLowerCase()
                : '';
              return n.toLowerCase().includes(f);
            })
            .map((labelValue) => {
              return (
                <MenuItem
                  key={labelValue}
                  value={labelValue}
                  onClick={() => onSelectedLabelValue(labelName, labelValue)}
                  className={
                    isLabelInQuery(query, labelName, labelValue) ? 'active' : ''
                  }
                >
                  {labelValue}
                </MenuItem>
              );
            })
        );
      }

      default:
      case 'failed':
      case 'pristine': {
        return null;
      }
    }
  };

  const GetFilter = (label: string) => {
    return (
      <FocusableItem>
        {({ ref }) => (
          <Input
            ref={ref}
            name="Search Tags Input"
            type="text"
            placeholder="Type a tag"
            value={filter[label] || ''}
            onChange={(e) =>
              setFilter({
                ...filter,
                [label]: e.target.value,
              })
            }
          />
        )}
      </FocusableItem>
    );
  };

  const Items = Object.entries(tags).map(([tag, tagValues]) => {
    return (
      <SubMenu
        key={tag}
        overflow="auto"
        position="initial"
        onChange={(open) => {
          // we are opening the menu for the first time
          if (open && tagValues.type !== 'loaded') {
            onSelectedLabel(tag);
          }
        }}
        label={() => (
          <span
            className="tag-content"
            aria-hidden
            onClick={() => onSelectedLabel(tag)}
          >
            {tag}
          </span>
        )}
      >
        {GetFilter(tag)}
        <MenuGroup takeOverflow>{GetTagValues(tag, tagValues)}</MenuGroup>
      </SubMenu>
    );
  });

  return <Dropdown label="Select Tag">{Items}</Dropdown>;
}

function appendLabelToQuery(query: string, label: string, labelValue: string) {
  const case1Regexp = new RegExp(`${label}=.+?(\\}|,)`);
  if (query.match(case1Regexp)) {
    return query.replace(case1Regexp, `${label}="${labelValue}"$1`);
  }
  if (query.indexOf('{}') !== -1) {
    return query.replace('}', `${label}="${labelValue}"}`);
  }
  if (query.indexOf('}') !== -1) {
    return query.replace('}', `, ${label}="${labelValue}"}`);
  }

  console.warn('TODO: handle this case');
  return query;
}

// Identifies whether a label is in a query or not
function isLabelInQuery(query: string, label: string, labelValue: string) {
  return query.includes(`${label}="${labelValue}"`);
}

export default TagsBar;
