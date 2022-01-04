import { createContext } from 'react';
import { randomId } from './helpers';

const Context = createContext();

const INTERPOLATION_DURATION = 60;

const DEFAULT_STATE = {
  allFilters: undefined,
  ducksoup: undefined,
  started: false,
  filters: [],
};

const initializeState = () => {
  const savedState = localStorage.getItem("state");
  if (savedState !== null) return JSON.parse(savedState);
  return DEFAULT_STATE;
}

export const initialState = initializeState();

const saveAndReturn = (state) => {
  console.log(state);
  setTimeout(() => localStorage.setItem("state", JSON.stringify(state)), 10);
  return state;
}

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
      return saveAndReturn(state);
    }
    case "addFilter": {
      if (state.allFilters) {
        const toAdd = state.allFilters.find((f) => f.display === action.payload);
        if (toAdd) {
          // important: clone and assign an id
          const newFilter = newFilterInstance(toAdd);
          return saveAndReturn({ ...state, filters: [...state.filters, newFilter] });
        }
      }
      return state;
    }
    case "removeFilter": {
      const newFilters = state.filters.filter((f) => f.id !== action.payload);
      return saveAndReturn({ ...state, filters: newFilters });
    }
    case "start":
      return { ...state, started: true };
    case "attachPlayer":
      return { ...state, ducksoup: action.payload };
    case "setAllFilters":
      return { ...state, allFilters: action.payload };
    default:
      return state;
  }
}

export default Context;