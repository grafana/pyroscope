import React, { useState, useEffect } from "react";
import { connect } from "react-redux";
import "react-dom";
import { Menu, SubMenu, MenuItem, MenuButton } from "@szhsin/react-menu";

import { fetchTags, fetchTagValues } from "../redux/actions";
import history from "../util/history";

function TagsBar(props) {
  const [tagsValue, setTagsValue] = useState("");

  const loadTagValues = (tag) => {
    if (
      props.tags[tag] &&
      !props.tags[tag].length &&
      props.tagValuesLoading !== tag
    ) {
      props.fetchTagValues(tag);
    }
  };

  useEffect(() => {
    props.fetchTags();
  }, []);

  useEffect(() => {
    history.push(`/?${tagsValue.replace(/[{}]/g, "")}`);
  }, [tagsValue]);

  return (
    <div className="tags-bar rc-menu-container--theme-dark">
      <Menu
        menuButton={<MenuButton>Select Tag</MenuButton>}
        theming="dark"
        keepMounted
      >
        {Object.keys(props.tags).map((tag) => (
          <SubMenu
            value={tag}
            key={tag}
            label={(e) => {
              if (
                !props.tags[tag].length &&
                e.active &&
                props.tagValuesLoading !== tag
              )
                loadTagValues(tag);
              return tag;
            }}
            onChange={(e) => loadTagValues(e.value)}
            className="active"
          >
            {props.tagValuesLoading === tag ? (
              <MenuItem>Loading...</MenuItem>
            ) : (
              props.tags[tag].map((tagValue) => (
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
