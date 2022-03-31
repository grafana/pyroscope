import React, { useState, useEffect, useRef } from 'react';
import Button from '@webapp/ui/Button';
import 'react-dom';
import { TagsState } from '@webapp/redux/reducers/continuous';
import Dropdown, {
  SubMenu,
  MenuItem,
  FocusableItem,
  MenuGroup,
} from '@webapp/ui/Dropdown';
import { Prism } from '@webapp/util/prism';
import styles from './TagsBar.module.css';

interface TagsBarProps {
  onSetQuery: (q: string) => void;
  onSelectedLabel: (label: string, query: string) => void;
  query: string;
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
              onSetQuery(newQuery);
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
          onSetQuery(q);
        }}
      />
    </div>
  );
}

interface QueryInputProps {
  initialQuery: string;
  onSubmit: (query: string) => void;
  //  className?: string;
}

function QueryInput({ initialQuery, onSubmit }: QueryInputProps) {
  const [query, setQuery] = useState(initialQuery);
  const codeRef = useRef<HTMLElement>(null);

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

  return (
    <form
      className="tags-query"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(query);
      }}
    >
      <pre className="tags-highlighted language-promql" aria-hidden="true">
        <code
          className="language-promql"
          id="highlighting-content"
          ref={codeRef}
        >
          {query}
        </code>
      </pre>
      <input
        className="tags-input"
        type="text"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
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
  query: string;
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
          <input
            ref={ref}
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
