import React, { useState, useEffect } from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import Button from '@ui/Button';
import 'react-dom';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  selectContinuousState,
  fetchTags,
} from '@pyroscope/redux/reducers/continuous';
import Dropdown, { SubMenu, MenuItem, FocusableItem } from '@ui/Dropdown';
import {
  //  fetchTags,
  fetchTagValues,
  setQuery,
  abortFetchTags,
  abortFetchTagValues,
} from '../redux/actions';
import '../util/prism';
import styles from './TagsBar.module.css';

function TagsBar({ actions, tags, tagValuesLoading }) {
  const dispatch = useAppDispatch();
  const { query } = useAppSelector(selectContinuousState);

  const [queryVal, setQuery] = useState(query);
  const [filter, setFilter] = useState({});

  useEffect(() => {
    dispatch(fetchTags(query));
  }, [query]);

  const submitTagsValue = (newValue) => {
    actions.setQuery(newValue);
  };

  const inputOnChange = (v) => {
    setQuery(v);
    window.Prism.highlightElement(
      document.getElementById('highlighting-content')
    );
  };

  useEffect(() => {
    if (window.Prism) {
      window.Prism.highlightElement(
        document.getElementById('highlighting-content')
      );
    }
  }, [queryVal]);

  const loadTagValues = (tag) => {
    actions.fetchTagValues(query, tag);
  };
  useEffect(
    () =>
      // since fetchTagValues may be running
      actions.abortFetchTagValues,
    []
  );

  const onTagsValueChange = (tagKey, tagValue) => {
    let newQuery;
    const case1Regexp = new RegExp(`${tagKey}=.+?(\\}|,)`);
    if (queryVal.match(case1Regexp)) {
      newQuery = queryVal.replace(case1Regexp, `${tagKey}="${tagValue}"$1`);
    } else if (queryVal.indexOf('{}') !== -1) {
      newQuery = queryVal.replace('}', `${tagKey}="${tagValue}"}`);
    } else if (queryVal.indexOf('}') !== -1) {
      newQuery = queryVal.replace('}', `, ${tagKey}="${tagValue}"}`);
    } else {
      console.warn('TODO: handle this case');
    }
    actions.setQuery(newQuery);
  };

  return (
    <div className="tags-bar _rc-menu-container--theme-dark">
      <Dropdown label="Select Tag">
        {Object.keys(tags).length === 0 ? (
          <MenuItem>No tags available</MenuItem>
        ) : (
          []
        )}
        {Object.keys(tags).map((tag) => (
          <SubMenu
            value={tag}
            key={tag}
            overflow="auto"
            position="initial"
            label={(e) => (
              <span
                className="tag-content"
                aria-hidden
                onClick={() => {
                  if (!tags[tag].length && tagValuesLoading !== tag)
                    loadTagValues(tag);
                }}
              >
                {tag}
              </span>
            )}
            className="active"
          >
            {tags && tags[tag] && tags[tag].length > 1 && (
              <FocusableItem>
                {({ ref }) => (
                  <input
                    ref={ref}
                    type="text"
                    placeholder="Type a tag"
                    value={filter[tag] || ''}
                    onChange={(e) =>
                      setFilter({
                        ...filter,
                        [tag]: e.target.value,
                      })
                    }
                  />
                )}
              </FocusableItem>
            )}

            {tagValuesLoading === tag ? (
              <MenuItem>Loading...</MenuItem>
            ) : (
              tags[tag]
                .filter((t) => {
                  const f = filter[tag] ? filter[tag].trim().toLowerCase() : '';
                  return t.toLowerCase().includes(f);
                })
                .map((tagValue) => (
                  <MenuItem
                    key={tagValue}
                    value={tagValue}
                    onClick={(e) => onTagsValueChange(tag, e.value)}
                    className={
                      queryVal.includes(`${tag}="${tagValue}"`) ? 'active' : ''
                    }
                  >
                    {tagValue}
                  </MenuItem>
                ))
            )}
          </SubMenu>
        ))}
      </Dropdown>
      <form
        className="tags-query"
        onSubmit={(e) => {
          e.preventDefault();
          submitTagsValue(queryVal);
        }}
      >
        <pre className="tags-highlighted language-promql" aria-hidden="true">
          <code className="language-promql" id="highlighting-content">
            {queryVal}
          </code>
        </pre>
        <input
          className="tags-input"
          type="text"
          value={queryVal}
          onChange={(e) => inputOnChange(e.target.value)}
          onBlur={(e) => submitTagsValue(queryVal)}
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
    </div>
  );
}

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchTags,
      fetchTagValues,
      abortFetchTags,
      setQuery,
    },
    dispatch
  ),
});

const mapStateToProps = (state) => ({
  ...state.root,
});
export default connect(mapStateToProps, mapDispatchToProps)(TagsBar);
