import type maplibregl from "maplibre-gl";

export const OFFLINE_MAP_STYLE: maplibregl.StyleSpecification = {
  version: 8,
  glyphs: "/offline-fonts/{fontstack}/{range}.pbf",
  sources: {
    osm: {
      type: "vector",
      tiles: ["/offline-tiles/{z}/{x}/{y}.pbf"],
      minzoom: 0,
      maxzoom: 14,
    },
    buildings: {
      type: "vector",
      tiles: ["/offline-buildings/{z}/{x}/{y}.pbf"],
      minzoom: 13,
      maxzoom: 15,
    },
    marks: {
      type: "geojson",
      data: { type: "FeatureCollection", features: [] },
    },
  },
  layers: [
    { id: "background", type: "background", paint: { "background-color": "#eef0e9" } },
    {
      id: "landcover",
      type: "fill",
      source: "osm",
      "source-layer": "landcover",
      paint: {
        "fill-color": [
          "match", ["get", "class"],
          "wood", "#cadfb9",
          "forest", "#c4dcb4",
          "grass", "#d7e6c7",
          "farmland", "#ece8d2",
          "#e4e9dc",
        ],
        "fill-opacity": 0.72,
      },
    },
    {
      id: "landuse",
      type: "fill",
      source: "osm",
      "source-layer": "landuse",
      paint: {
        "fill-color": [
          "match", ["get", "class"],
          "residential", "#e9e5dd",
          "park", "#cfe3c2",
          "forest", "#c8dfba",
          "industrial", "#e4e1da",
          "#ebe7df",
        ],
        "fill-opacity": 0.75,
      },
    },
    {
      id: "water",
      type: "fill",
      source: "osm",
      "source-layer": "water",
      paint: { "fill-color": "#a9d2df", "fill-outline-color": "#91bdcf" },
    },
    {
      id: "waterway",
      type: "line",
      source: "osm",
      "source-layer": "waterway",
      paint: { "line-color": "#aad3df", "line-width": ["interpolate", ["linear"], ["zoom"], 10, 1, 16, 4] },
    },
    {
      id: "buildings",
      type: "fill",
      source: "osm",
      "source-layer": "building",
      minzoom: 13,
      paint: {
        "fill-color": ["match", ["get", "class"], "residential", "#d9d1c5", "#d5cabb"],
        "fill-outline-color": "#bfb3a0",
        "fill-opacity": 0.9,
      },
    },
    {
      id: "buildings-detailed",
      type: "fill",
      source: "buildings",
      "source-layer": "buildings",
      minzoom: 13,
      paint: {
        "fill-color": ["match", ["get", "class"], "residential", "#d3c9ba", "#cec1b0"],
        "fill-outline-color": "#a89a85",
        "fill-opacity": 0.95,
      },
    },
    {
      id: "railways",
      type: "line",
      source: "osm",
      "source-layer": "transportation",
      filter: ["==", ["get", "class"], "rail"],
      paint: {
        "line-color": "#9aa3ad",
        "line-width": ["interpolate", ["linear"], ["zoom"], 12, 0.7, 16, 1.6],
        "line-dasharray": [3, 2],
      },
    },
    {
      id: "road-casing",
      type: "line",
      source: "osm",
      "source-layer": "transportation",
      paint: {
        "line-color": "#d8c6ae",
        "line-width": ["interpolate", ["linear"], ["zoom"], 10, 1.2, 14, 4, 18, 14],
        "line-gap-width": ["interpolate", ["linear"], ["zoom"], 10, 0.3, 14, 1.5, 18, 6],
      },
    },
    {
      id: "roads",
      type: "line",
      source: "osm",
      "source-layer": "transportation",
      paint: {
        "line-color": [
          "match", ["get", "class"],
          "motorway", "#e5a15a",
          "trunk", "#eeb06b",
          "primary", "#f0c26f",
          "secondary", "#f5d48d",
          "tertiary", "#f8e7b8",
          "#ffffff",
        ],
        "line-width": ["interpolate", ["linear"], ["zoom"], 10, 0.5, 14, 2.2, 18, 9],
      },
    },
    {
      id: "boundary",
      type: "line",
      source: "osm",
      "source-layer": "boundary",
      paint: { "line-color": "#9ca3af", "line-width": 1, "line-dasharray": [4, 3] },
    },
    {
      id: "place-city",
      type: "symbol",
      source: "osm",
      "source-layer": "place",
      filter: ["==", ["get", "class"], "city"],
      minzoom: 11,
      maxzoom: 15,
      layout: {
        "text-field": ["coalesce", ["get", "name:ru"], ["get", "name"]],
        "text-font": ["Noto Sans Bold"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 11, 14, 14, 19, 18, 24],
        "text-letter-spacing": 0.02,
        "text-max-width": 8,
        "text-padding": 8,
      },
      paint: {
        "text-color": "#26323f",
        "text-halo-color": "#ffffff",
        "text-halo-width": 2,
        "text-halo-blur": 0.4,
      },
    },
    {
      id: "place-suburb",
      type: "symbol",
      source: "osm",
      "source-layer": "place",
      filter: ["match", ["get", "class"], ["suburb", "quarter", "neighbourhood"], true, false],
      minzoom: 12,
      maxzoom: 16,
      layout: {
        "text-field": ["coalesce", ["get", "name:ru"], ["get", "name"]],
        "text-font": ["Noto Sans Regular"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 12, 10, 15, 13],
        "text-letter-spacing": 0.08,
        "text-transform": "uppercase",
        "text-max-width": 9,
        "text-padding": 5,
      },
      paint: {
        "text-color": "#707b88",
        "text-halo-color": "#ffffff",
        "text-halo-width": 1.5,
        "text-halo-blur": 0.5,
      },
    },
    {
      id: "road-labels-major",
      type: "symbol",
      source: "osm",
      "source-layer": "transportation_name",
      filter: ["match", ["get", "class"], ["motorway", "trunk", "primary", "secondary", "tertiary"], true, false],
      minzoom: 13,
      layout: {
        "text-field": ["coalesce", ["get", "name:ru"], ["get", "name"]],
        "text-font": ["Noto Sans Regular"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 13, 10, 17, 13],
        "symbol-placement": "line",
        "text-rotation-alignment": "map",
        "text-pitch-alignment": "viewport",
        "text-max-angle": 35,
        "symbol-spacing": 350,
        "text-padding": 4,
      },
      paint: {
        "text-color": "#44505d",
        "text-halo-color": "#ffffff",
        "text-halo-width": 1.6,
      },
    },
    {
      id: "road-labels-minor",
      type: "symbol",
      source: "osm",
      "source-layer": "transportation_name",
      filter: ["match", ["get", "class"], ["minor", "service"], true, false],
      minzoom: 15,
      layout: {
        "text-field": ["coalesce", ["get", "name:ru"], ["get", "name"]],
        "text-font": ["Noto Sans Regular"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 15, 9.5, 18, 12],
        "symbol-placement": "line",
        "text-rotation-alignment": "map",
        "text-pitch-alignment": "viewport",
        "text-max-angle": 30,
        "symbol-spacing": 300,
        "text-padding": 3,
      },
      paint: {
        "text-color": "#6b7280",
        "text-halo-color": "#ffffff",
        "text-halo-width": 1.4,
      },
    },
    {
      id: "water-labels",
      type: "symbol",
      source: "osm",
      "source-layer": "water_name",
      minzoom: 12,
      layout: {
        "text-field": ["coalesce", ["get", "name:ru"], ["get", "name"]],
        "text-font": ["Noto Sans Italic"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 12, 10, 17, 14],
        "text-max-width": 9,
        "text-padding": 4,
      },
      paint: {
        "text-color": "#4f7f92",
        "text-halo-color": "#eff5f2",
        "text-halo-width": 1.5,
      },
    },
    {
      id: "poi-labels",
      type: "symbol",
      source: "osm",
      "source-layer": "poi",
      filter: ["match", ["get", "class"], ["hospital", "school", "college", "university", "railway", "bus", "museum", "theatre", "park", "government"], true, false],
      minzoom: 16,
      layout: {
        "text-field": ["coalesce", ["get", "name:ru"], ["get", "name"]],
        "text-font": ["Noto Sans Regular"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 16, 9.5, 18, 11.5],
        "text-max-width": 9,
        "text-padding": 3,
        "symbol-sort-key": ["get", "rank"],
      },
      paint: {
        "text-color": "#4b5563",
        "text-halo-color": "#ffffff",
        "text-halo-width": 1.4,
      },
    },
    {
      id: "house-numbers",
      type: "symbol",
      source: "osm",
      "source-layer": "housenumber",
      minzoom: 17,
      layout: {
        "text-field": ["get", "housenumber"],
        "text-font": ["Noto Sans Regular"],
        "text-size": ["interpolate", ["linear"], ["zoom"], 17, 9, 18.5, 11],
        "text-max-width": 4,
        "text-padding": 2,
      },
      paint: {
        "text-color": "#6b5b4a",
        "text-halo-color": "#f4f1ea",
        "text-halo-width": 1.5,
      },
    },
    {
      id: "marks-halo",
      type: "circle",
      source: "marks",
      paint: {
        "circle-radius": ["interpolate", ["linear"], ["zoom"], 10, 7, 14, 10, 18, 14],
        "circle-color": "#ffffff",
        "circle-opacity": 0.92,
      },
    },
    {
      id: "marks-selected",
      type: "circle",
      source: "marks",
      filter: ["==", ["get", "id"], ""],
      paint: {
        "circle-radius": ["interpolate", ["linear"], ["zoom"], 10, 8, 14, 13, 18, 18],
        "circle-color": "#1d4ed8",
        "circle-opacity": 0.28,
        "circle-stroke-color": "#1d4ed8",
        "circle-stroke-width": 2,
      },
    },
    {
      id: "marks-circles",
      type: "circle",
      source: "marks",
      paint: {
        "circle-radius": ["interpolate", ["linear"], ["zoom"], 10, 4.5, 14, 7, 18, 10],
        "circle-color": ["match", ["get", "event_type"], "check_out", "#d97706", "#15803d"],
        "circle-stroke-color": ["case", ["get", "outside_office"], "#dc2626", "#ffffff"],
        "circle-stroke-width": 2,
      },
    },
  ],
};



export type OfficeMapItem = {
  id: string;
  name: string;
  lat: number;
  lon: number;
  radius_m: number;
  is_active: boolean;
};

type OfficeProperties = { id: string; name: string; radius_m: number; is_active: boolean };

export type OfficePointCollection = {
  type: "FeatureCollection";
  features: Array<{
    type: "Feature";
    geometry: { type: "Point"; coordinates: [number, number] };
    properties: OfficeProperties;
  }>;
};

export type OfficeZoneCollection = {
  type: "FeatureCollection";
  features: Array<{
    type: "Feature";
    geometry: { type: "Polygon"; coordinates: number[][][] };
    properties: OfficeProperties;
  }>;
};

export function officePointCollection(offices: OfficeMapItem[]): OfficePointCollection {
  return {
    type: "FeatureCollection",
    features: offices.map((office) => ({
      type: "Feature",
      geometry: { type: "Point", coordinates: [office.lon, office.lat] },
      properties: { id: office.id, name: office.name, radius_m: office.radius_m, is_active: office.is_active },
    })),
  };
}

export function officeZoneCollection(offices: OfficeMapItem[]): OfficeZoneCollection {
  const points = 64;
  return {
    type: "FeatureCollection",
    features: offices.map((office) => {
      const ring: number[][] = [];
      const latRad = office.lat * Math.PI / 180;
      for (let index = 0; index <= points; index += 1) {
        const angle = (index / points) * Math.PI * 2;
        const lat = office.lat + (office.radius_m * Math.sin(angle)) / 110540;
        const lon = office.lon + (office.radius_m * Math.cos(angle)) / (111320 * Math.cos(latRad));
        ring.push([lon, lat]);
      }
      return {
        type: "Feature",
        geometry: { type: "Polygon", coordinates: [ring] },
        properties: { id: office.id, name: office.name, radius_m: office.radius_m, is_active: office.is_active },
      };
    }),
  };
}
