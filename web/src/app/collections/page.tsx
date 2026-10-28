"use client";

import React, { useEffect, useState } from "react";
import { useI18n } from "@/i18n/provider";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Database, FolderPlus, Save, Trash2 } from "lucide-react";

interface CollectionItem {
  collection: string;
  id: string;
  data: unknown;
  updated_at: string;
}

export default function CollectionsPage() {
  const { dict } = useI18n();

  const [collections, setCollections] = useState<string[]>([]);
  const [activeCollection, setActiveCollection] = useState<string>("projects");
  const [newCollectionName, setNewCollectionName] = useState<string>("");
  const [items, setItems] = useState<CollectionItem[]>([]);
  const [docId, setDocId] = useState<string>("item-1");
  const [jsonBody, setJsonBody] = useState<string>(
    '{\n  "title": "Domain Record",\n  "status": "active"\n}'
  );
  const [errorMsg, setErrorMsg] = useState<string>("");

  const loadCollections = () => {
    fetch("/api/collections")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data && Array.isArray(data.collections)) {
          setCollections(data.collections);
        }
      })
      .catch(() => {});
  };

  const loadItems = (colName: string) => {
    if (!colName) return;
    fetch(`/api/collections/${colName}`)
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data && Array.isArray(data.items)) {
          setItems(data.items);
        }
      })
      .catch(() => {});
  };

  useEffect(() => {
    loadCollections();
    loadItems(activeCollection);
  }, [activeCollection]);

  const handleOpenCollection = (e: React.FormEvent) => {
    e.preventDefault();
    const target = newCollectionName.trim();
    if (!target) return;
    setActiveCollection(target);
    setNewCollectionName("");
  };

  const handleSaveDocument = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg("");
    let parsed: unknown;
    try {
      parsed = JSON.parse(jsonBody);
    } catch {
      setErrorMsg(dict.collections.invalidJson);
      return;
    }
    const targetId = docId.trim() || `item-${Date.now()}`;
    const res = await fetch(
      `/api/collections/${activeCollection}/${targetId}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(parsed),
      }
    );
    if (res.ok) {
      loadCollections();
      loadItems(activeCollection);
    }
  };

  const handleSelectItem = (it: CollectionItem) => {
    setDocId(it.id);
    setJsonBody(JSON.stringify(it.data, null, 2));
    setErrorMsg("");
  };

  const handleDeleteItem = async (id: string) => {
    await fetch(`/api/collections/${activeCollection}/${id}`, {
      method: "DELETE",
    }).catch(() => {});
    loadItems(activeCollection);
    loadCollections();
  };

  return (
    <div className="max-w-6xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold">{dict.collections.title}</h1>
          <p className="text-sm text-zinc-400">{dict.collections.subtitle}</p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Badge>collection_list</Badge>
          <Badge>collection_get</Badge>
          <Badge>collection_put</Badge>
          <Badge>collection_delete</Badge>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">
        {/* Collections Sidebar */}
        <Card className="lg:col-span-1">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm flex items-center gap-2">
              <Database className="h-4 w-4 text-sky-400" />
              {dict.collections.collectionsList}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <form onSubmit={handleOpenCollection} className="flex gap-1.5">
              <input
                value={newCollectionName}
                onChange={(e) => setNewCollectionName(e.target.value)}
                placeholder={dict.collections.newCollectionPlaceholder}
                className="flex-1 min-w-0 rounded border border-zinc-800 bg-zinc-950 px-2.5 py-1.5 text-xs text-zinc-100"
              />
              <Button size="sm" type="submit">
                <FolderPlus className="h-3.5 w-3.5" />
                {dict.collections.openCollection}
              </Button>
            </form>

            <div className="space-y-1">
              {Array.from(new Set([activeCollection, ...collections])).map(
                (col) => (
                  <button
                    key={col}
                    type="button"
                    onClick={() => setActiveCollection(col)}
                    className={`w-full text-left rounded-md px-3 py-2 text-xs font-mono cursor-pointer transition-colors ${
                      col === activeCollection
                        ? "bg-sky-500/15 text-sky-300 border border-sky-500/30"
                        : "text-zinc-300 hover:bg-zinc-800/60"
                    }`}
                  >
                    /api/collections/{col}
                  </button>
                )
              )}
            </div>
          </CardContent>
        </Card>

        {/* Document List & JSON Editor */}
        <div className="lg:col-span-3 grid grid-cols-1 md:grid-cols-2 gap-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">
                {dict.collections.itemsTitle}{" "}
                <span className="font-mono text-sky-300">
                  {activeCollection}
                </span>
              </CardTitle>
              <CardDescription className="text-xs">
                {dict.collections.cardSubtitle}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 max-h-[440px] overflow-y-auto">
              {items.length === 0 ? (
                <div className="text-xs text-zinc-500 py-8 text-center">
                  {dict.collections.noItems}
                </div>
              ) : (
                items.map((it) => (
                  <div
                    key={it.id}
                    onClick={() => handleSelectItem(it)}
                    className="rounded-lg border border-zinc-800 bg-zinc-950/70 p-3 space-y-1.5 cursor-pointer hover:border-sky-500/40"
                  >
                    <div className="flex items-center justify-between">
                      <Badge className="font-mono text-[11px]">{it.id}</Badge>
                      <button
                        type="button"
                        title={dict.collections.deleteDoc}
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteItem(it.id);
                        }}
                        className="text-zinc-500 hover:text-rose-400 cursor-pointer"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <pre className="text-[11px] text-zinc-300 overflow-x-auto max-h-28">
                      {JSON.stringify(it.data, null, 2)}
                    </pre>
                  </div>
                ))
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-mono">
                PUT /api/collections/{activeCollection}/{docId || "{id}"}
              </CardTitle>
              <CardDescription className="text-xs">
                {dict.collections.jsonBodyLabel}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleSaveDocument} className="space-y-3">
                <input
                  value={docId}
                  onChange={(e) => setDocId(e.target.value)}
                  placeholder={dict.collections.docIdPlaceholder}
                  className="w-full rounded border border-zinc-800 bg-zinc-950 px-3 py-1.5 text-xs font-mono text-zinc-100"
                />
                <textarea
                  rows={10}
                  value={jsonBody}
                  onChange={(e) => setJsonBody(e.target.value)}
                  className="w-full rounded border border-zinc-800 bg-zinc-950 p-3 text-xs font-mono text-zinc-100 focus:outline-none focus:border-sky-500"
                />
                {errorMsg && (
                  <div className="text-xs text-rose-400">{errorMsg}</div>
                )}
                <Button type="submit" className="w-full">
                  <Save className="h-4 w-4" />
                  {dict.collections.saveDoc}
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
