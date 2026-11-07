"use client";

import "maplibre-gl/dist/maplibre-gl.css";
import "./FuelMap.css"; // depois do CSS da biblioteca: os ajustes do tema valem mais

import type { FeatureCollection, Point, Polygon, MultiPolygon } from "geojson";
import {
  AttributionControl,
  getVersion,
  MapLibreMap,
  NavigationControl,
  Popup,
  setWorkerUrl,
  type ExpressionSpecification,
  type GeoJSONSource,
  type LngLatBoundsLike,
  type MapLayerMouseEvent,
} from "maplibre-gl";
import { useEffect, useRef, useState } from "react";

import type { MunicipalPrice } from "@/lib/api";
import { formatPrice, mapClassOf, type MapClass } from "@/lib/fuels";
import { cssToken, useColorScheme } from "../use-color-scheme";
import { MAP_HEIGHT_CLASS } from "./sizes";

// O worker do MapLibre é servido de public/vendor (scripts/copy-maplibre-worker.mjs).
// O ?v= troca a URL a cada versão, para o navegador não usar um worker antigo.
setWorkerUrl(`/vendor/maplibre-gl-worker.mjs?v=${getVersion()}`);

/** Oeste, sul, leste, norte: o Brasil inteiro. */
const BRAZIL: [number, number, number, number] = [-74, -33.8, -34.7, 5.3];
const FIT_PADDING = 24;

const LOCALE = {
  "AttributionControl.ToggleAttribution": "Mostrar ou esconder os créditos",
  "Map.Title": "Mapa",
  "NavigationControl.ZoomIn": "Aproximar",
  "NavigationControl.ZoomOut": "Afastar",
  "Popup.Close": "Fechar",
  "CooperativeGesturesHandler.WindowsHelpText": "Use Ctrl + rolagem para aproximar o mapa",
  "CooperativeGesturesHandler.MacHelpText": "Use ⌘ + rolagem para aproximar o mapa",
  "CooperativeGesturesHandler.MobileHelpText": "Use dois dedos para mover o mapa",
};

/** O que cada ponto carrega (nomes curtos: viajam para o worker do mapa). */
interface PointProps {
  name: string;
  uf: string;
  avg: number;
  min: number;
  max: number;
  n: number;
  color: string; // cor da faixa de preço, já resolvida para o tema atual
}

interface UFProps {
  uf: string;
}

interface Props {
  municipalities: MunicipalPrice[];
  scale: MapClass[];
  uf: string | null;
  unit: string;
}

/** Cores do mapa, lidas das variáveis CSS do tema: o WebGL não entende var(--x). */
function readMapColors() {
  return {
    surface: cssToken("--surface"),
    land: cssToken("--map-land"),
    landSelected: cssToken("--map-land-selected"),
    border: cssToken("--map-border"),
    steps: [1, 2, 3, 4, 5].map((i) => cssToken(`--seq-${i}`)),
  };
}

/**
 * Um ponto por município, com a cor da faixa de preço. A cor vai pronta no
 * dado (e não numa expressão do estilo): ao trocar o tema, os pontos são
 * remontados, o que é barato para ~400 municípios.
 */
function toGeoJSON(
  municipalities: MunicipalPrice[],
  scale: MapClass[],
  steps: string[],
): FeatureCollection<Point, PointProps> {
  return {
    type: "FeatureCollection",
    features: municipalities.map((m) => ({
      type: "Feature",
      id: m.ibge_code,
      geometry: { type: "Point", coordinates: [m.longitude, m.latitude] },
      properties: {
        name: m.name,
        uf: m.uf,
        avg: m.avg_price,
        min: m.min_price,
        max: m.max_price,
        n: m.n_stations,
        color: steps[scale[mapClassOf(m.avg_price, scale)].step - 1],
      },
    })),
  };
}

/**
 * Conteúdo do balão. Montado com nós do DOM e textContent, nunca com HTML
 * em texto: um nome de município nunca vira código na página.
 */
function popupContent(p: PointProps, unit: string): HTMLElement {
  const line = (text: string, className: string) => {
    const el = document.createElement("div");
    el.className = className;
    el.textContent = text;
    return el;
  };
  const root = document.createElement("div");
  root.append(
    line(formatPrice(p.avg), "text-[15px] font-semibold tabular-nums"),
    line(`${p.name} (${p.uf})`, "text-ink"),
    line(
      p.n === 1 ? `1 posto, ${unit}` : `${p.n} postos, de ${formatPrice(p.min)} a ${formatPrice(p.max)}`,
      "text-ink-2 tabular-nums",
    ),
  );
  return root;
}

/**
 * Mapa dos preços por município: um ponto por município, colorido pela faixa
 * de preço, sobre o contorno dos estados. Não usa nenhum serviço de mapas: o
 * fundo é só o contorno das UFs (public/geo/ufs.json, do IBGE).
 *
 * O mapa é criado uma vez; quando os filtros mudam, os efeitos abaixo só
 * trocam os dados, as cores e o enquadramento.
 */
export default function FuelMap({ municipalities, scale, uf, unit }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MapLibreMap | null>(null);
  const ufBounds = useRef<Record<string, LngLatBoundsLike>>({});
  const [ready, setReady] = useState(false);
  const scheme = useColorScheme();

  // 1. Cria o mapa e as camadas (os pontos chegam no efeito 2).
  useEffect(() => {
    const controller = new AbortController();
    // As camadas já nascem com as cores do tema: sem cor, o MapLibre pinta de preto.
    const colors = readMapColors();
    const map = new MapLibreMap({
      container: container.current!,
      style: {
        version: 8,
        sources: {},
        layers: [{ id: "background", type: "background", paint: { "background-color": colors.surface } }],
      },
      bounds: BRAZIL,
      fitBoundsOptions: { padding: FIT_PADDING },
      minZoom: 2,
      maxZoom: 10,
      renderWorldCopies: false,
      // Mapa de dados: girar e inclinar só atrapalham.
      dragRotate: false,
      pitchWithRotate: false,
      touchPitch: false,
      // A rolagem da página não fica presa no mapa: zoom com Ctrl + rolagem,
      // e no celular o mapa se move com dois dedos.
      cooperativeGestures: true,
      attributionControl: false,
      locale: LOCALE,
    });
    map.touchZoomRotate.disableRotation();
    map.keyboard.disableRotation();
    map.addControl(new NavigationControl({ showCompass: false }), "top-right");
    map.addControl(new AttributionControl({ compact: true, customAttribution: "Fronteiras: IBGE · Preços: ANP" }));
    mapRef.current = map;

    const ufs = fetch("/geo/ufs.json", { signal: controller.signal }).then(
      (res) => res.json() as Promise<FeatureCollection<Polygon | MultiPolygon, UFProps>>,
    );
    map.once("load", async () => {
      try {
        const geo = await ufs;
        for (const f of geo.features) {
          if (f.bbox) ufBounds.current[f.properties.uf] = f.bbox as [number, number, number, number];
        }
        map.addSource("ufs", { type: "geojson", data: geo });
        map.addSource("municipalities", { type: "geojson", data: { type: "FeatureCollection", features: [] } });
        map.addLayer({ id: "uf-fill", type: "fill", source: "ufs", paint: { "fill-color": colors.land } });
        map.addLayer({ id: "uf-line", type: "line", source: "ufs", paint: { "line-color": colors.border } });
        map.addLayer({
          id: "municipalities",
          type: "circle",
          source: "municipalities",
          paint: {
            // Pontos crescem com o zoom; o anel da cor do fundo separa os vizinhos.
            "circle-radius": ["interpolate", ["linear"], ["zoom"], 3, 3.5, 6, 6, 9, 9],
            "circle-color": ["get", "color"],
            "circle-stroke-width": 1,
            "circle-stroke-color": colors.surface,
          },
        });
        setReady(true);
      } catch {
        // Abortado (o mapa saiu da tela) ou falha de rede: o mapa fica sem dados.
      }
    });

    return () => {
      controller.abort();
      map.remove();
      mapRef.current = null;
    };
  }, []);

  // 2. Dados e cores: a cada troca de filtro e de tema claro/escuro.
  useEffect(() => {
    const map = mapRef.current;
    if (!ready || !map || !scheme) return;
    const colors = readMapColors();
    const inState: ExpressionSpecification = ["==", ["get", "uf"], uf ?? ""];
    // Com uma UF escolhida, os pontos de fora ficam apagados (e desenhados por baixo).
    const opacity: ExpressionSpecification | number = uf ? ["case", inState, 1, 0.2] : 1;

    map.setPaintProperty("background", "background-color", colors.surface);
    map.setPaintProperty("uf-fill", "fill-color", ["case", inState, colors.landSelected, colors.land]);
    map.setPaintProperty("uf-line", "line-color", colors.border);
    map.setPaintProperty("uf-line", "line-width", ["case", inState, 1.5, 0.6]);
    (map.getSource("municipalities") as GeoJSONSource).setData(toGeoJSON(municipalities, scale, colors.steps));
    map.setPaintProperty("municipalities", "circle-stroke-color", colors.surface);
    map.setPaintProperty("municipalities", "circle-opacity", opacity);
    map.setPaintProperty("municipalities", "circle-stroke-opacity", opacity);
    map.setLayoutProperty("municipalities", "circle-sort-key", uf ? ["case", inState, 1, 0] : 0);
  }, [ready, municipalities, scale, uf, scheme]);

  // 3. Enquadra a UF escolhida (ou o Brasil).
  useEffect(() => {
    const map = mapRef.current;
    if (!ready || !map) return;
    map.fitBounds((uf && ufBounds.current[uf]) || BRAZIL, { padding: FIT_PADDING, maxZoom: 7.5, duration: 600 });
  }, [ready, uf]);

  // 4. Balão com o preço: ao passar o mouse, ou ao tocar no ponto no celular.
  useEffect(() => {
    const map = mapRef.current;
    if (!ready || !map) return;
    const popup = new Popup({ closeButton: false, closeOnClick: false, offset: 10, maxWidth: "260px" });
    const show = (e: MapLayerMouseEvent) => {
      const feature = e.features?.[0];
      if (!feature) return;
      map.getCanvas().style.cursor = "pointer";
      const [lon, lat] = (feature.geometry as Point).coordinates;
      popup
        .setLngLat([lon, lat])
        .setDOMContent(popupContent(feature.properties as PointProps, unit))
        .addTo(map);
    };
    const hide = () => {
      map.getCanvas().style.cursor = "";
      popup.remove();
    };
    const hideOutside = (e: MapLayerMouseEvent) => {
      if (map.queryRenderedFeatures(e.point, { layers: ["municipalities"] }).length === 0) hide();
    };
    map.on("mousemove", "municipalities", show);
    map.on("click", "municipalities", show);
    map.on("mouseleave", "municipalities", hide);
    map.on("click", hideOutside);
    return () => {
      map.off("mousemove", "municipalities", show);
      map.off("click", "municipalities", show);
      map.off("mouseleave", "municipalities", hide);
      map.off("click", hideOutside);
      popup.remove();
    };
  }, [ready, unit]);

  return <div ref={container} className={`${MAP_HEIGHT_CLASS} overflow-hidden rounded-lg`} />;
}
