import React from "react";
import { configure, mount } from "enzyme";
import { Provider } from "react-redux";
import configureMockStore from "redux-mock-store";
import Adapter from "enzyme-adapter-react-16";
import { ShortcutProvider } from "react-keybind";

import ShortcutsModal from "../javascript/components/ShortcutsModal";
import Sidebar from "../javascript/components/Sidebar";

const mockStore = configureMockStore();
configure({ adapter: new Adapter() });

const store = mockStore({});

describe("ShortcutsModal", () => {
  it("When shortcuts are pressed, a shortcuts modal should appears", () => {
    const wrapper = mount(
      <Provider store={store}>
        <ShortcutProvider>
          <Sidebar />
        </ShortcutProvider>
      </Provider>
    );
    
    wrapper.find("button").last().simulate("click");
    expect(wrapper.find(ShortcutsModal).length).toBe(1);

  });
});

