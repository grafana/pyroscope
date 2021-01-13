import React from "react";
import Label from "../javascript/components/Label";
import { configure } from "enzyme";
import Adapter from "enzyme-adapter-react-16";
import { shallow, mount, render } from "enzyme";
import { Provider } from "react-redux";
import configureStore from "redux-mock-store";

const mockStore = configureStore();
configure({ adapter: new Adapter() });
let store, wrapper;
const initialState = { label: { name: "Expensive Process", value: 32 } };

beforeEach(() => {
  store = mockStore(initialState);
  wrapper = mount(
    <Provider store={store}>
      <Label />
    </Provider>
  );
});

describe("Label is correctly rendered", () => {
  it("Correctly renders name", () => {
    expect(
      wrapper.contains(
        <span className="label-name">{initialState.label.name}</span>
      )
    ).toBe(true);
  });
  it("Correctly renders value", () => {
    expect(
      wrapper.contains(
        <span className="label-value">{initialState.label.value}</span>
      )
    ).toBe(true);
  });
});
