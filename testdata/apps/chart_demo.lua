-- Chart.js demo (kforms_enhancements.md §3).
-- Run with: ./KALUA run testdata/apps/chart_demo.lua

function main()
    -- Dashboard with a shaded line chart, a bar chart and a donut chart.
    -- The timer fires every 3s and pushes new data via k.chart.set_data.
    k.form.new("dashboard", {title = "Sales Dashboard", layout = "grid"})

    k.ctrl.chart("dashboard", "sales_trend", {
        type = "line",
        title = "Monthly Sales",
        labels = {"Jan", "Feb", "Mar", "Apr", "May", "Jun"},
        datasets = {
            {
                label = "Revenue",
                data = {12000, 19000, 15000, 25000, 22000, 30000},
                borderColor = "#1976d2",
                backgroundColor = "rgba(25, 118, 210, 0.1)",
                fill = true,
                tension = 0.3
            },
            {
                label = "Orders",
                data = {120, 180, 150, 250, 220, 300},
                borderColor = "#d32f2f",
                backgroundColor = "rgba(211, 47, 47, 0.1)",
                fill = false,
                tension = 0.3
            }
        },
        options = {
            scales = {
                y = {beginAtZero = true, title = {display = true, text = "Amount"}}
            }
        }
    })

    k.ctrl.chart("dashboard", "category_sales", {
        type = "bar",
        title = "Sales by Category",
        labels = {"Electronics", "Clothing", "Home", "Sports"},
        datasets = {{
            label = "Q1 Sales",
            data = {45000, 32000, 28000, 19000},
            backgroundColor = {"#1976d2", "#388e3c", "#f57c00", "#7b1fa2"}
        }}
    })

    k.ctrl.chart("dashboard", "share", {
        type = "doughnut",
        title = "Revenue Share",
        labels = {"Electronics", "Clothing", "Home", "Sports"},
        datasets = {{
            label = "Share",
            data = {45, 32, 28, 19},
            backgroundColor = {"#1976d2", "#388e3c", "#f57c00", "#7b1fa2"}
        }},
        legendPosition = "right"
    })

    -- Interactivity: act on chart clicks and legend clicks.
    k.form.on("dashboard", "sales_trend", "chart_click", function(di, idx, val)
        k.print("sales_trend click: dataset", di, "point", idx, "value", val)
    end)
    k.form.on("dashboard", "category_sales", "chart_legend_click", function(di)
        k.print("category_sales legend click: dataset", di)
    end)

    -- Export the current line chart to a PNG data URL.
    k.ctrl.button("dashboard", "btn_export", {
        label = "Export line chart PNG",
        onclick = function()
            local img = k.chart.get_image("dashboard", "sales_trend")
            if img then
                k.print("chart image length:", #img)
                k.status_show("Captured " .. #img .. " bytes")
                k.timer_start("hide_status", 1500)
            end
        end
    })

    function hide_status()
        k.status_close()
    end

    -- Live update every 3 seconds: bump the revenue series by one point.
    local tick = 0
    function update_chart()
        tick = tick + 1
        local labels = {"Jan", "Feb", "Mar", "Apr", "May", "Jun"}
        local data = {12000, 19000, 15000, 25000, 22000, 30000}
        for i = 1, #data do
            data[i] = data[i] + tick * 500
        end
        k.chart.set_data("dashboard", "sales_trend", {
            labels = labels,
            datasets = {
                {
                    label = "Revenue",
                    data = data,
                    borderColor = "#1976d2",
                    backgroundColor = "rgba(25, 118, 210, 0.1)",
                    fill = true,
                    tension = 0.3
                },
                {
                    label = "Orders",
                    data = {120, 180, 150, 250, 220, 300},
                    borderColor = "#d32f2f",
                    type = "line",
                    fill = false,
                    tension = 0.3
                }
            }
        })
        if tick >= 10 then
            k.timer_stop("update_chart")
        end
    end
    k.timer_start("update_chart", 3000, true)

    k.form.show("dashboard")
end