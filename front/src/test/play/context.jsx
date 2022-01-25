import { createContext } from 'react';
import { randomId } from './helpers';

const Context = createContext();

const DEFAULT_DURATION = 60; // seconds
const INTERPOLATION_DURATION = 60; // ms

const DEFAULT_STATE = {
  ducksoup: undefined,
  started: false,
  record: false,
  duration: DEFAULT_DURATION,
  flatFilters: undefined,
  enabledFilters: [],
  groupedAudioFilters: undefined,
  groupedVideoFilters: undefined,
};

const initializeState = () => {
  const serialized = localStorage.getItem("state-v01");
  let saved;
  if (serialized !== null) saved = JSON.parse(serialized);
  return {...DEFAULT_STATE, ...saved};
}

export const initialState = initializeState();

const saveAndReturn = (state) => {
  const { enabledFilters } = state;
  setTimeout(() => localStorage.setItem("state-v01", JSON.stringify({ enabledFilters })), 10);
  return state;
}

const newFilterInstance = (template) => {
  // deep copy with JSON API to avoid clashes of subobjects (controls)
  // when using same filter several times 
  const newFilter = JSON.parse(JSON.stringify(template)); 
  newFilter.id = randomId();
  if (newFilter.controls) {
    for (let i = 0; i < newFilter.controls.length; i++) {
      newFilter.controls[i].current = newFilter.controls[i].default;
    }
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
      const filterToUpdate = state.enabledFilters.find((f) => f.id === id);
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
          return saveAndReturn({ ...state, enabledFilters: [...state.enabledFilters, newFilter] });
        }
      }
      return state;
    }
    case "removeFilter": {
      const newFilters = state.enabledFilters.filter((f) => f.id !== action.payload);
      return saveAndReturn({ ...state, enabledFilters: newFilters });
    }
    case "start":
      return { ...state, started: true };
    case "toggleRecord":
      return { ...state, record: !state.record };
    case "setDuration":
      let newDuration = action.payload;
      if(isNaN(newDuration)) {
        newDuration = DEFAULT_DURATION;
      } else if (newDuration < 1) {
        newDuration = 1;
      } else if (newDuration > 600) {
        newDuration = 600;
      }
      return { ...state, duration: newDuration };
    case "stop":
      if (state.ducksoup) {
        state.ducksoup.stop();
      }
      return { ...state, started: false };
    case "attachPlayer":
      return { ...state, ducksoup: action.payload };
    case "setFilters":
      const flatAudioFilters = action.payload.filter(({ type }) => type === "audio");
      const flatVideoFilters = action.payload.filter(({ type }) => type === "video");
      return {
        ...state,
        flatFilters: action.payload,
        groupedAudioFilters: groupBy(flatAudioFilters, "category"),
        groupedVideoFilters: groupBy(flatVideoFilters, "category"),
      };
    default:
      return state;
  }
}

export default Context;