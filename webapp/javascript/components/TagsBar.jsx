import React, { useState, useEffect } from "react";
import { bindActionCreators } from "redux";
import { connect } from "react-redux";
import "react-dom";
import { Menu, SubMenu, MenuItem, MenuButton } from "@szhsin/react-menu";

import { fetchTags, fetchTagValues, setQuery } from "../redux/actions";
import "../util/prism";

function TagsBar({ query, actions, tags, tagValuesLoading }) {
  const [queryVal, setQuery] = useState(query);

  useEffect(() => {
    setQuery(query);
  }, [query]);

  useEffect(() => {
    actions.fetchTags(query);
  }, [query]);

  const submitTagsValue = (newValue) => {
    actions.setQuery(newValue);
  };

  const inputOnChange = (v) => {
    setQuery(v);
    window.Prism.highlightElement(
      document.getElementById("highlighting-content")
    );
  };

  useEffect(() => {
    if (window.Prism) {
      window.Prism.highlightElement(
        document.getElementById("highlighting-content")
      );
    }
  }, [queryVal]);

  const loadTagValues = (tag) => {
    actions.fetchTagValues(query, tag);
  };

  const onTagsValueChange = (tagKey, tagValue) => {
    let newQuery;
    const case1Regexp = new RegExp(`${tagKey}=.+?(\\}|,)`);
    if (queryVal.match(case1Regexp)) {
      newQuery = queryVal.replace(case1Regexp, `${tagKey}="${tagValue}"$1`);
    } else if (queryVal.indexOf("{}") !== -1) {
      newQuery = queryVal.replace("}", `${tagKey}="${tagValue}"}`);
    } else if (queryVal.indexOf("}") !== -1) {
      newQuery = queryVal.replace("}", `, ${tagKey}="${tagValue}"}`);
    } else {
      console.warn("TODO: handle this case");
    }
    actions.setQuery(newQuery);
  };

  return (
    <div className="tags-bar rc-menu-container--theme-dark">
      <Menu
        menuButton={<MenuButton>Select Tag</MenuButton>}
        theming="dark"
        keepMounted
      >
        {Object.keys(tags).length === 0 ? (
          <MenuItem>No tags available</MenuItem>
        ) : (
          []
        )}
        {Object.keys(tags).map((tag) => (
          <SubMenu
            value={tag}
            key={tag}
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
            {tagValuesLoading === tag ? (
              <MenuItem>Loading...</MenuItem>
            ) : (
              tags[tag].map((tagValue) => (
                <MenuItem
                  key={tagValue}
                  value={tagValue}
                  onClick={(e) => onTagsValueChange(tag, e.value)}
                  className={
                    queryVal.includes(`${tag}="${tagValue}"`) ? "active" : ""
                  }
                >
                  {tagValue}
                </MenuItem>
              ))
            )}
          </SubMenu>
        ))}
      </Menu>
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
        <button className="btn tags-query-execute">Execute</button>
      </form>
    </div>
  );
}

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchTags,
      fetchTagValues,
      setQuery,
    },
    dispatch
  ),
});

export default connect((state) => state, mapDispatchToProps)(TagsBar);
