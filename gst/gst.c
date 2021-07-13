#include "gst.h"

#include <gst/app/gstappsrc.h>

typedef struct SampleHandlerUserData
{
    int pipelineId;
} SampleHandlerUserData;

GMainLoop *gstreamer_main_loop = NULL;
void gstreamer_start_mainloop(void)
{
    gstreamer_main_loop = g_main_loop_new(NULL, FALSE);

    g_main_loop_run(gstreamer_main_loop);
}

static gboolean gstreamer_bus_call(GstBus *bus, GstMessage *msg, gpointer data)
{
    GstElement* pipeline = (GstElement*) data;
    switch (GST_MESSAGE_TYPE(msg))
    {
    case GST_MESSAGE_EOS: {
        g_print("[gst.c] end of stream\n");

        gst_element_set_state(pipeline, GST_STATE_NULL);
        break;
    }
    case GST_MESSAGE_ERROR:
    {
        gchar *debug;
        GError *error;

        g_printerr ("[gst.c] error received from element %s: %s\n",
            GST_OBJECT_NAME (msg->src), error->message);

        g_printerr ("[gst.c] debugging information: %s\n", debug ? debug : "none");

        g_free(debug);
        g_error_free(error);
        
        gst_element_set_state(pipeline, GST_STATE_NULL);
        break;
    }
    default:
        //g_print("got message %s\n", gst_message_type_get_name (GST_MESSAGE_TYPE (msg)));
        break;
    }

    return TRUE;
}

GstFlowReturn gstreamer_new_sample_handler(GstElement *object, gpointer user_data)
{
    GstSample *sample = NULL;
    GstBuffer *buffer = NULL;
    gpointer copy = NULL;
    gsize copy_size = 0;
    SampleHandlerUserData *s = (SampleHandlerUserData *)user_data;

    g_signal_emit_by_name(object, "pull-sample", &sample);
    if (sample)
    {
        buffer = gst_sample_get_buffer(sample);
        if (buffer)
        {
            gst_buffer_extract_dup(buffer, 0, gst_buffer_get_size(buffer), &copy, &copy_size);
            goHandleNewSample(s->pipelineId, copy, copy_size, GST_BUFFER_DURATION(buffer));
        }
        gst_sample_unref(sample);
    }

    return GST_FLOW_OK;
}

GstElement *gstreamer_parse_pipeline(char *pipeline)
{
    gst_init(NULL, NULL);
    GError *error = NULL;
    return gst_parse_launch(pipeline, &error);
}

float gstreamer_get_fx_property(GstElement *pipeline, char *elName, char *elProp) {
    GstElement* fx;
    gfloat value;
 
    fx = gst_bin_get_by_name(GST_BIN(pipeline), elName);
    
    if(fx) {
        g_object_get(fx, elProp, &value, NULL);
        gst_object_unref(fx);
    }

    return value;
}

void gstreamer_set_fx_property(GstElement *pipeline, char *elName, char *elProp, float elValue)
{
    GstElement* fx;

    fx = gst_bin_get_by_name(GST_BIN(pipeline), elName);
    
    if(fx) {
        g_object_set(fx, elProp, elValue, NULL);
        gst_object_unref(fx);
    }
}

void gstreamer_start_pipeline(GstElement *pipeline, int pipelineId)
{
    SampleHandlerUserData *s = calloc(1, sizeof(SampleHandlerUserData));
    s->pipelineId = pipelineId;

    GstBus *bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline));

    gst_bus_add_watch(bus, gstreamer_bus_call, pipeline);
    gst_object_unref(bus);

    GstElement *appsink = gst_bin_get_by_name(GST_BIN(pipeline), "sink");
    g_object_set(appsink, "emit-signals", TRUE, NULL);
    g_signal_connect(appsink, "new-sample", G_CALLBACK(gstreamer_new_sample_handler), s);
    gst_object_unref(appsink);

    gst_element_set_state(pipeline, GST_STATE_PLAYING);
}

void gstreamer_stop_pipeline(GstElement *pipeline)
{
    gst_element_send_event(pipeline, gst_event_new_eos());
}

void gstreamer_push_buffer(GstElement *pipeline, void *buffer, int len)
{
    GstElement *src = gst_bin_get_by_name(GST_BIN(pipeline), "src");
    if (src != NULL)
    {
        gpointer p = g_memdup(buffer, len);
        GstBuffer *buffer = gst_buffer_new_wrapped(p, len);
        gst_app_src_push_buffer(GST_APP_SRC(src), buffer);
        gst_object_unref(src);
    }
}