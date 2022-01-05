import { createContext } from 'react';
import { randomId } from './helpers';

const Context = createContext();

const INTERPOLATION_DURATION = 60;

const DEFAULT_STATE = {
  flatFilters: undefined,
  groupedFilters: undefined,
  ducksoup: undefined,
  started: false,
  filters: [],
};

const initializeState = () => {
  const serialized = localStorage.getItem("state-v01");
  let saved;
  if (serialized !== null) saved = JSON.parse(serialized);
  return {...DEFAULT_STATE, ...saved};
}

export const initialState = initializeState();

const saveAndReturn = (state) => {
  const { filters } = state;
  setTimeout(() => localStorage.setItem("state-v01", JSON.stringify({ filters })), 10);
  return state;
}

const newFilterInstance = (template) => {
  // deep copy with JSON API to avoid clashes of subobjects (controls)
  // when using same filter several times 
  const newFilter = JSON.parse(JSON.stringify(template)); 
  newFilter.id = randomId();
  for (let i = 0; i < newFilter.controls.length; i++) {
    newFilter.controls[i].current = newFilter.controls[i].default;
  }
  return newFilter;
}

const groupBy = (xs, key) => xs.reduce(function(rv, x) {
    (rv[x[key]] = rv[x[key]] || []).push(x);
    return rv;
}, {});

export const reducer = (state, action) => {
  switch (action.type) {
    case "newControlValue": {
      const { id, gst, kind, value } = action.payload;
      if (state.ducksoup) {
        state.ducksoup.polyControlFx(id, gst, kind, value, INTERPOLATION_DURATION);
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
      if (state.flatFilters) {
        const toAdd = state.flatFilters.find((f) => f.display === action.payload);
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
    case "stop":
      return { ...state, started: false };
    case "attachPlayer":
      return { ...state, ducksoup: action.payload };
    case "setFilters":
      const grouped = groupBy(action.payload, "category");
      return { ...state, flatFilters: action.payload, groupedFilters: grouped };
    default:
      return state;
  }
}

export default Context;