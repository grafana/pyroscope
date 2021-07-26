import React, { useState, useEffect } from "react";
import { connect } from "react-redux";
import "react-dom";
import { Menu, SubMenu, MenuItem, MenuButton } from "@szhsin/react-menu";

import { fetchTags, fetchTagValues } from "../redux/actions";
import history from "../util/history";

function TagsBar({ tags, fetchTags, fetchTagValues, tagValuesLoading }) {
  const [tagsValue, setTagsValue] = useState("");

  const loadTagValues = (tag) => {
    if (tags[tag] && !tags[tag].length && tagValuesLoading !== tag) {
      fetchTagValues(tag);
    }
  };

  useEffect(() => {
    fetchTags();
  }, []);

  useEffect(() => {
    const url = new URL(window.location.href);
    Object.keys(tags).forEach((tag) => {
      if (url.search.includes(tag)) {
        loadTagValues(tag);
        setTagsValue(`{${tag}=${url.searchParams.get(tag)}}`);
      }
    });
  }, [tags]);

  useEffect(() => {
    const [name, value] = tagsValue.replace(/[{}]/g, "").split("=");
    const url = new URL(window.location.href);
    if (value) {
      url.searchParams.set(name, value);
    } else {
      url.searchParams.delete(name);
    }
    history.push(url.search);
  }, [tagsValue]);

  return (
    <div className="tags-bar rc-menu-container--theme-dark">
      <Menu
        menuButton={<MenuButton>Select Tag</MenuButton>}
        theming="dark"
        keepMounted
      >
        {Object.keys(tags).map((tag) => (
          <SubMenu
            value={tag}
            key={tag}
            label={(e) => {
              if (!tags[tag].length && e.active && tagValuesLoading !== tag)
                loadTagValues(tag);
              return tag;
            }}
            onChange={(e) => loadTagValues(e.value)}
            className="active"
          >
            {tagValuesLoading === tag ? (
              <MenuItem>Loading...</MenuItem>
            ) : (
              tags[tag].map((tagValue) => (
                <MenuItem
                  key={tagValue}
                  value={tagValue}
                  onClick={(e) => setTagsValue(`{${tag}=${e.value}}`)}
                  className={tagsValue.includes(tagValue) ? "active" : ""}
                >
                  {tagValue}
                </MenuItem>
              ))
            )}
          </SubMenu>
        ))}
      </Menu>
      <input
        className="tags-input"
        type="text"
        value={tagsValue}
        onChange={(e) => setTagsValue(e.target.value)}
      />
    </div>
  );
}

export default connect((state) => state, { fetchTags, fetchTagValues })(
  TagsBar
);
