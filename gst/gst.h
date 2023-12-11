#ifndef GST_H
#define GST_H

#include <glib.h>
#include <gst/gst.h>
#include <stdint.h>
#include <stdlib.h>

extern void goWriteAudio(char *id, void *buffer, int bufferLen);
extern void goWriteVideo(char *id, void *buffer, int bufferLen);
extern void goDeletePipeline(char *id);
extern void goRequestKeyFrame(char *id);
extern void goBusLog(char *id, char *msg, char *el);
extern void goDebugLog(int level, char *file, char *function,int line, char *msg);

void gstStartMainLoop(void);
GstElement *gstParsePipeline(char *pipelineStr, char *id);
void gstStartPipeline(GstElement *pipeline, gboolean audioOnly);
void gstStopPipeline(GstElement *pipeline);
void gstSrcPush(GstElement *pipeline, char *src, void *buffer, int len);
void gstSendPLI(GstElement *pipeline);

// get/set props
float gstGetPropFloat(GstElement *pipeline, char *elName, char *elProp);
void gstSetPropFloat(GstElement *pipeline, char *elName, char *elProp, float elValue);
double gstGetPropDouble(GstElement *pipeline, char *name, char *prop);
void gstSetPropDouble(GstElement *pipeline, char *name, char *prop, double value);
gint gstGetPropInt(GstElement *pipeline, char *elName, char *elProp);
void gstSetPropInt(GstElement *pipeline, char *elName, char *elProp, gint elValue);
guint64 gstGetPropUint64(GstElement *pipeline, char *name, char *prop);
void gstSetPropUint64(GstElement *pipeline, char *name, char *prop, guint64 value);
void gstSetPropString(GstElement *pipeline, char *name, char *prop, char *value);

#endif