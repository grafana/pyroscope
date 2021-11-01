import React from 'react';
import { configure, mount } from 'enzyme';
import { Provider } from 'react-redux';
import configureMockStore from 'redux-mock-store';
import Adapter from 'enzyme-adapter-react-16';
import { ShortcutProvider } from 'react-keybind';

import ShortcutsModal from '../javascript/components/ShortcutsModal';
import Sidebar from '../javascript/components/Sidebar';
import { MemoryRouter } from 'react-router';

const mockStore = configureMockStore();
configure({ adapter: new Adapter() });

const store = mockStore({});

describe('ShortcutsModal', () => {
  it('When shortcuts are pressed, a shortcuts modal should appears', () => {
    const wrapper = mount(
      <Provider store={store}>
        <ShortcutProvider>
          <MemoryRouter>
            <Sidebar />
          </MemoryRouter>
        </ShortcutProvider>
      </Provider>
    );

    wrapper.find('#tests-shortcuts-btn').last().simulate('click');
    expect(wrapper.find(ShortcutsModal).length).toBe(1);
  });
});
