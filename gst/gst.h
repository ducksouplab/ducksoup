#ifndef GST_H
#define GST_H

#include <glib.h>
#include <gst/gst.h>
#include <stdint.h>
#include <stdlib.h>

extern void goNewSampleCallback(char *id, void *buffer, int bufferLen, int pts);
extern void goForceKeyUnitCallback(char *id);
extern void goStopCallback(char *message);

GstElement *gstreamer_parse_pipeline(char *pipelineStr, char *id);
void gstreamer_start_pipeline(GstElement *pipeline);
void gstreamer_stop_pipeline(GstElement *pipeline);
void gstreamer_push_buffer(GstElement *pipeline, void *buffer, int len);
void gstreamer_push_buffer(GstElement *pipeline, void *buffer, int len);
float gstreamer_get_property_float(GstElement *pipeline, char *elName, char *elProp);
void gstreamer_set_property_float(GstElement *pipeline, char *elName, char *elProp, float elValue);
gint gstreamer_get_property_int(GstElement *pipeline, char *elName, char *elProp);
void gstreamer_set_property_int(GstElement *pipeline, char *elName, char *elProp, gint elValue);
void gstreamer_start_mainloop(void);

#endif