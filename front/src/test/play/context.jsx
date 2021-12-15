import { createContext } from 'react';
import { randomId } from './helpers';

const Context = createContext();

const INTERPOLATION_DURATION = 60;

export const initialState = {
  allFilters: undefined,
  ducksoup: undefined,
  running: false,
  filters: [],
};

const newFilterInstance = (template) => {
  const newFilter = { ...template, id: randomId() };
  for (let i = 0; i < newFilter.controls.length; i++) {
    newFilter.controls[i].current = newFilter.controls[i].default;
  }
  return newFilter;
}

export const reducer = (state, action) => {
  switch (action.type) {
    case "newControlValue": {
      const { id, gst, value } = action.payload;
      if (state.ducksoup) {
        state.ducksoup.controlFx(id, gst, value, INTERPOLATION_DURATION);
      }
      // ugly in place edit
      const filterToUpdate = state.filters.find((f) => f.id === id);
      if (filterToUpdate) {
        const controlToUpdate = filterToUpdate.controls.find((c) => c.gst === gst);
        if (controlToUpdate) {
          controlToUpdate.current = value;
        }
      }
      return state;
    }
    case "addFilter": {
      if (state.allFilters) {
        const toAdd = state.allFilters.find((f) => f.display === action.payload);
        if (toAdd) {
          // important: clone and assign an id
          const newFilter = newFilterInstance(toAdd);
          return { ...state, filters: [...state.filters, newFilter] };
        }
      }
      return state;
    }
    case "isRunning":
      return { ...state, running: true };
    case "attachPlayer":
      return { ...state, ducksoup: action.payload };
    case "setAllFilters":
      return { ...state, allFilters: action.payload };
    default:
      return state;
  }
}

export default Context;