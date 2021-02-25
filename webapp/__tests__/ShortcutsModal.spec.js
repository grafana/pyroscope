import React from "react";
import { configure, mount } from "enzyme";
import { Provider } from "react-redux";
import configureMockStore from "redux-mock-store";
import Adapter from "enzyme-adapter-react-16";
import { ShortcutProvider } from "react-keybind";
import renderer from "react-test-renderer";

import ShortcutsModal from "../javascript/components/ShortcutsModal";
import Sidebar from "../javascript/components/Sidebar";

const mockStore = configureMockStore();
configure({ adapter: new Adapter() });

const store = mockStore({});

describe("ShortcutsModal", () => {
  it("render correctly ShortcutsModal component", () => {
    const ShortcutsModalComponent = renderer
      .create(
        <Provider store={store}>
          <ShortcutProvider>
            <Sidebar />
          </ShortcutProvider>
        </Provider>
      )
      .toJSON();
    expect(ShortcutsModalComponent).toMatchSnapshot();
  });
  it("When shortcuts are pressed, a shortcuts modal should appears", () => {
    const wrapper = mount(
      <Provider store={store}>
        <ShortcutProvider>
          <Sidebar />
        </ShortcutProvider>
      </Provider>
    );
    wrapper.find("button").at(1).simulate("click");
    expect(wrapper.find(ShortcutsModal).length).toBe(1);
  });
});
