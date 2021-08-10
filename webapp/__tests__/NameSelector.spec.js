import React from "react";
import renderer from "react-test-renderer";

import { configure, shallow, mount, render } from "enzyme";
import { Provider } from "react-redux";
import configureMockStore from "redux-mock-store";
import Adapter from "enzyme-adapter-react-16";

import NameSelector from "../javascript/components/NameSelector";

const mockStore = configureMockStore();
configure({ adapter: new Adapter() });

const store = mockStore({});

const props = {
  query: "hotrod.golang.customer",
  names: ["hotrod.golang.customer", "hotrod.golang.driver"],
};

describe("NameSelector", () => {
  it("render correctly NameSelector component", () => {
    const NameSelectorComponent = renderer
      .create(
        <Provider store={store}>
          <NameSelector {...props} />
        </Provider>
      )
      .toJSON();
    expect(NameSelectorComponent).toMatchSnapshot();
  });
  it("When the name in the Applications dropdown menu changes, the page re-renders", () => {
    const wrapper = mount(
      <Provider store={store}>
        <NameSelector {...props} />
      </Provider>
    );

    wrapper.find("select").simulate("mouseDown");
    expect(wrapper.find("option").length).toEqual(3);
    wrapper.find("option").at(2).simulate("click", null);
  });
});
